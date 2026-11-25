package controller

import (
	"context"
	"mofufans/internal/avatar"
	"mofufans/internal/bilibili"
	"mofufans/internal/database"
	"strings"
	"time"
	"unicode/utf8"
)

const memberProfileRefreshTimeout = 2 * time.Second

// refreshLoginProfile is called only after a successful password check. The
// fresh public card is separate from the seven-day administrative profile cache.
// Both the card lookup and safe avatar download share this short deadline.
func (ctl *AccountController) refreshLoginProfile(parent context.Context, authenticated database.User) {
	ctx, cancel := context.WithTimeout(parent, memberProfileRefreshTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	// Best-effort refresh must not queue a successful login behind other lookups.
	limits := ctl.limits()
	select {
	case limits.network <- struct{}{}:
		defer func() { <-limits.network }()
	default:
		return
	}
	reader := ctl.LoginProfileReader
	if reader == nil {
		reader = bilibili.NewClient()
	}
	profile, err := reader.PublicSignature(ctx, authenticated.BilibiliUID)
	if err != nil || ctx.Err() != nil || profile.MID != authenticated.BilibiliUID || !utf8.ValidString(profile.Name) || strings.TrimSpace(profile.Name) == "" || utf8.RuneCountInString(profile.Name) > 255 || len(profile.Face) > 1024 {
		return
	}
	updates := map[string]any{}
	if profile.Name != authenticated.BilibiliName {
		updates["bilibili_name"] = profile.Name
	}
	if profile.Face != "" {
		store := ctl.AvatarStorage
		if store == nil {
			store = avatar.NewStore()
		}
		// Store validates the HTTPS image host, redirects, size and media type.
		// Its UID/content-hash filename stays the same when the picture is unchanged.
		stored, saveErr := store.Save(ctx, authenticated.BilibiliUID, profile.Face)
		if saveErr == nil && stored != "" && len(stored) <= 1024 && stored != authenticated.BilibiliAvatar {
			updates["bilibili_avatar"] = stored
		}
	}
	if len(updates) == 0 {
		return
	}
	// A competing login or account change must not be overwritten by a stale
	// response. Never Save the whole User: it includes credentials and privileges.
	database.DB.Model(&database.User{}).Where(
		"bilibili_uid = ? AND password_hash = ? AND auth_version = ? AND is_disabled = ? AND bilibili_name = ? AND bilibili_avatar = ?",
		authenticated.BilibiliUID, authenticated.PasswordHash, authenticated.AuthVersion, false, authenticated.BilibiliName, authenticated.BilibiliAvatar,
	).Updates(updates)
	// Keep both prior and unused new files here. Historical question snapshots or
	// another concurrent writer can still reference them; a login must not delete
	// shared images based on an unlocked reference count.
}

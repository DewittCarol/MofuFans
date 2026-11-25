"use client";
import { useSiteTheme } from "@/lib/site-theme";
import { AISettingsPanel } from "@/components/ai-settings";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import Link from "next/link";
import { BilibiliVerificationSettings } from "@/components/bilibili-verification-settings";
import { SettingsNavigation } from "@/components/settings-navigation";
import { InstallationSettingsPanel } from "@/components/installation-settings";
import { getSettings, updateSettings, type CursorTheme } from "@/lib/api";

export default function SettingsPage() {
 const {theme}=useSiteTheme();
  const [requireVerifiedUserToPost, setRequireVerifiedUserToPost] = useState(false);
  const [accountsEnabled, setAccountsEnabled] = useState(true);
  const [allowRepeatedReactions, setAllowRepeatedReactions] = useState(false);
  const [voyagePublicEnabled, setVoyagePublicEnabled] = useState(false);
  const [petDefaultExpanded, setPetDefaultExpanded] = useState(false);
  const [cursorTheme, setCursorTheme] = useState("system");
  const [cursorThemes, setCursorThemes] = useState<CursorTheme[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [isSuccess, setIsSuccess] = useState(false);

  useEffect(() => {
    const loadSettings = async () => {
      try {
        const res = await getSettings();
        if (res.code === 200) {
          setRequireVerifiedUserToPost(!!res.data.require_verified_user_to_post);
          setAccountsEnabled(!res.data.disable_member_accounts);
          setAllowRepeatedReactions(!!res.data.allow_repeated_reactions);
        setVoyagePublicEnabled(!!res.data.voyage_public_enabled);
          setPetDefaultExpanded(!!res.data.pet_default_expanded);
          setCursorTheme(res.data.cursor_theme || "mofu");
          setCursorThemes(res.data.cursor_themes || []);
        } else {
          setMessage(res.message || "加载失败");
        }
      } catch {
        setMessage("加载失败");
      } finally {
        setIsLoading(false);
      }
    };

    loadSettings();
  }, []);

  const handleSave = async () => {
    setIsSaving(true);
    setMessage("");
    setIsSuccess(false);
    try {
      const res = await updateSettings({
        cursor_theme: cursorTheme,
        require_verified_user_to_post: requireVerifiedUserToPost,
        disable_member_accounts: !accountsEnabled,
        allow_repeated_reactions: allowRepeatedReactions,
        voyage_public_enabled: voyagePublicEnabled,
        pet_default_expanded: petDefaultExpanded,
      });
      if (res.code === 200) {
        setRequireVerifiedUserToPost(!!res.data.require_verified_user_to_post);
        setAccountsEnabled(!res.data.disable_member_accounts);
        setAllowRepeatedReactions(!!res.data.allow_repeated_reactions);
        setVoyagePublicEnabled(!!res.data.voyage_public_enabled);
        setPetDefaultExpanded(!!res.data.pet_default_expanded);
          setCursorTheme(res.data.cursor_theme || "mofu");
          setCursorThemes(res.data.cursor_themes || []);
        setMessage("保存成功");
        setIsSuccess(true);
      } else {
        setMessage(res.message || "保存失败");
      }
    } catch {
      setMessage("保存失败");
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <SettingsNavigation>
    <div className="space-y-8">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">
          设置
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">品牌、配色、背景、鼠标、表情素材与页脚备案均可在 <Link className="text-primary underline underline-offset-4" href="/dashboard/themes/">主题管理</Link> 中配置。</p>
      </header>

      <InstallationSettingsPanel />

      <Card id="submission-settings" tabIndex={-1} aria-label="投稿权限">
        <CardHeader>
          <CardTitle>投稿权限</CardTitle>
          <CardDescription>修改后点击“保存投稿与互动设置”。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox id="member-accounts-enabled" checked={accountsEnabled}
              onCheckedChange={checked => { setAccountsEnabled(!!checked); if (!checked) setRequireVerifiedUserToPost(false); }} disabled={isLoading || isSaving} />
            <div className="space-y-1">
              <Label htmlFor="member-accounts-enabled">开启前台登录 / 注册</Label>
              <p className="text-sm text-muted-foreground">关闭后暂停用户登录、注册和用户中心，投稿使用匿名方式。已有地址和领取记录保留，后台登录不受影响。</p>
            </div>
          </div>

          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox
              id="require-verified-user"
              checked={requireVerifiedUserToPost}
              onCheckedChange={(checked) => setRequireVerifiedUserToPost(!!checked)}
              disabled={isLoading || isSaving || !accountsEnabled}
            />
            <div className="space-y-1">
              <Label htmlFor="require-verified-user">投稿需要已验证用户</Label>
              <p className="text-sm text-muted-foreground">
                开启后仅已完成 B 站验证并登录的用户可以投稿。
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card id="voyage-settings" tabIndex={-1} aria-label="舰礼展示">
        <CardHeader><CardTitle>舰礼展示</CardTitle></CardHeader>
        <CardContent>
          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox id="voyage-public-enabled" checked={voyagePublicEnabled}
              onCheckedChange={checked => setVoyagePublicEnabled(!!checked)} disabled={isLoading || isSaving} />
            <div className="space-y-1">
              <Label htmlFor="voyage-public-enabled">在前台展示舰礼</Label>
              <p className="text-sm text-muted-foreground">开启后访客可查看已发布的舰礼；关闭后公开目录仅供管理员预览。已登录并完成签名验证的用户，仍可在用户中心查看本人已发布的领取批次和物流。点击“保存设置”后生效。</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card id="cursor-settings" tabIndex={-1} aria-label="鼠标指针">
        <CardHeader><CardTitle>鼠标指针</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <Label htmlFor="cursor-theme">全站使用的套装</Label>
          <Select value={cursorTheme} onValueChange={setCursorTheme} disabled={isLoading || isSaving}>
            <SelectTrigger id="cursor-theme" className="w-full max-w-sm"><SelectValue placeholder="选择鼠标套装" /></SelectTrigger>
            <SelectContent>
              {cursorThemes.map((theme) => <SelectItem key={theme.id} value={theme.id}>{theme.name}</SelectItem>)}
            </SelectContent>
          </Select>
          <p className="text-sm text-muted-foreground">保存后前台与后台都会自动切换，手机触屏不受影响。</p>
          {theme.brand.cursor_message&&<p className="text-sm text-muted-foreground">{theme.brand.cursor_message}</p>}
        </CardContent>
      </Card>

      <Card id="pet-settings" tabIndex={-1} aria-label="桌宠">
        <CardHeader><CardTitle>桌宠</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <Label htmlFor="pet-default-state">进入网站时的默认状态</Label>
          <Select value={petDefaultExpanded ? "expanded" : "collapsed"} onValueChange={value => setPetDefaultExpanded(value === "expanded")} disabled={isLoading || isSaving}>
            <SelectTrigger id="pet-default-state" className="w-full max-w-sm"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="expanded">展开</SelectItem><SelectItem value="collapsed">收起</SelectItem></SelectContent>
          </Select>
          <p className="text-sm text-muted-foreground">访客进入网站时按此设置显示，之后仍可自行展开或收起。收起时不加载模型，手机端不显示桌宠。点击“保存设置”后生效。</p>
        </CardContent>
      </Card>

      <Card id="reaction-settings" tabIndex={-1} aria-label="贴表情互动">
        <CardHeader><CardTitle>贴表情互动</CardTitle></CardHeader>
        <CardContent className="space-y-5">
          <div className="flex max-w-2xl items-start gap-3">
            <Checkbox id="allow-repeated-reactions" checked={allowRepeatedReactions}
              onCheckedChange={(checked) => setAllowRepeatedReactions(!!checked)} disabled={isLoading || isSaving} />
            <div className="space-y-1">
              <Label htmlFor="allow-repeated-reactions">允许反复贴表情</Label>
              <p className="text-sm text-muted-foreground">开启后可以在同一张卡片上多次贴表情，每次都会计数并触发动画；关闭后每个网络地址对每张卡片只能评价一次。点击下方“保存投稿与互动设置”后生效。</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <div id="save-settings" tabIndex={-1} aria-label="保存设置" className="flex flex-wrap items-center gap-3">
        <Button onClick={handleSave} disabled={isLoading || isSaving}>
          {isLoading ? "加载中..." : isSaving ? "保存中..." : "保存投稿与互动设置"}
        </Button>
        {message && (
          <p className={isSuccess ? "text-success" : "text-destructive"}>
            {message}
          </p>
        )}
      </div>
      <BilibiliVerificationSettings />
      <AISettingsPanel />
    </div>
    </SettingsNavigation>
  );
}

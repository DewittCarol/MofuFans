package controller

import (
	"mofufans/internal/database"
	"strings"
)

// The same lexicographic ordering as Get, with ID breaking otherwise equal rows.
// All column names come from getOrderBy, never from raw query parameters.
func questionPreceding(column, direction string, q database.Question) (string, []any) {
	values := map[string]any{"id": q.ID, "created_at": q.CreatedAt, "is_hide": q.IsHide,
		"is_rainbow": q.IsRainbow, "is_archive": q.IsArchive, "is_publish": q.IsPublish, "images_num": q.ImagesNum}
	columns, directions := []string{column}, []string{direction}
	if column != "is_archive" {
		columns = append(columns, "is_archive")
		directions = append(directions, "asc")
	}
	if column != "id" {
		columns = append(columns, "id")
		directions = append(directions, direction)
	}
	var alternatives []string
	var args []any
	for i, name := range columns {
		var terms []string
		for j := 0; j < i; j++ {
			terms = append(terms, columns[j]+" = ?")
			args = append(args, values[columns[j]])
		}
		comparison := " < ?"
		if directions[i] == "desc" {
			comparison = " > ?"
		}
		terms = append(terms, name+comparison)
		args = append(args, values[name])
		alternatives = append(alternatives, "("+strings.Join(terms, " AND ")+")")
	}
	return "(" + strings.Join(alternatives, " OR ") + ")", args
}

package app

import (
	"fmt"

	"skillctl/internal/source"
)

// importOutcome is one item's result plus whether an unresolved Store
// operation intent remains after the item. A true blocked stops every later
// Store mutation in the batch; the remaining selections still receive
// failed outcomes.
type importOutcome struct {
	result  ImportItemResult
	blocked bool
}

// blockedOutcome is the outcome of a selection that follows an item that
// left an unresolved Store operation intent: no Store mutation may run, so
// the item fails with CodeRecovery and a clear blocking message. The
// requested slug is preserved when it is already known (an explicit
// override, or a derivable default).
func blockedOutcome(sel importSelection) importOutcome {
	requested := ""
	if sel.Selector.Slug != nil {
		requested = *sel.Selector.Slug
	} else if slug, err := defaultSlug(sel.Entry.Name); err == nil {
		requested = slug
	}
	return importOutcome{result: ImportItemResult{
		Status: StatusFailed, RelativeDir: sel.Entry.RelativeDir,
		RequestedSlug: requested,
		ErrorCode:     CodeRecovery,
		ErrorMessage:  "a previous item left an unresolved Store operation; later Store writes are blocked until recovery succeeds",
	}}
}

// failedImport builds one failed item outcome with a stable error code and
// message. It carries no slug metadata; use failedImportMeta once the
// requested/final slug is known.
func failedImport(entry source.Entry, code, format string, args ...any) ImportItemResult {
	return ImportItemResult{
		Status: StatusFailed, RelativeDir: entry.RelativeDir,
		ErrorCode: code, ErrorMessage: fmt.Sprintf(format, args...),
	}
}

// failedImportMeta is failedImport plus the known identity of the item: the
// requested (and final) slug, the committed Skill id when one exists, and
// the replacement preview when the item targeted an existing managed Skill.
func failedImportMeta(entry source.Entry, slug string, skillID int64, replaces *Skill, code, format string, args ...any) ImportItemResult {
	item := failedImport(entry, code, format, args...)
	item.RequestedSlug = slug
	item.Slug = slug
	item.SkillID = skillID
	item.Replaces = replaces
	return item
}

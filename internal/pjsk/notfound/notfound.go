// Package notfound builds the typed "nothing matches" errors of the game
// data lookups (songs, cards, events, gacha). Lookups deep in the render
// layer do not know which region the user asked about, so they return the
// plain form; the command handler, which knows the region, upgrades it with
// InRegion. Both steps match by message ID, never by text.
package notfound

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

// Music reports that no song matches query (empty: no query to show).
func Music(query string) *usererror.Error {
	if query = strings.TrimSpace(query); query == "" {
		return usererror.Wrap(usererror.CodeNotFound, i18n.M("music.not_found_unspecified"), errors.New("music not found"))
	}
	return usererror.Wrap(usererror.CodeNotFound, i18n.M("music.not_found", i18n.Data{"Query": i18n.EchoQuery(query)}), fmt.Errorf("music not found: %s", query))
}

// MusicID reports that no song has id.
func MusicID(id int) *usererror.Error { return Music(strconv.Itoa(id)) }

// Card reports that no card matches query (empty: no query to show).
func Card(query string) *usererror.Error {
	if query = strings.TrimSpace(query); query == "" {
		return usererror.Wrap(usererror.CodeNotFound, i18n.M("card.not_found_unspecified"), errors.New("card not found"))
	}
	return usererror.Wrap(usererror.CodeNotFound, i18n.M("card.not_found", i18n.Data{"Query": i18n.EchoQuery(query)}), fmt.Errorf("card not found: %s", query))
}

// CardID reports that no card has id.
func CardID(id int) *usererror.Error { return Card(strconv.Itoa(id)) }

// Event reports that no event matches the query.
func Event() *usererror.Error {
	return usererror.Wrap(usererror.CodeNotFound, i18n.M("event.not_found"), errors.New("event not found"))
}

// Gacha reports that no gacha matches the query.
func Gacha() *usererror.Error {
	return usererror.Wrap(usererror.CodeNotFound, i18n.M("gacha.not_found"), errors.New("gacha not found"))
}

// InRegion upgrades a plain not-found error of this package to the form that
// names region and suggests a region prefix. fallbackQuery is shown when the
// lookup did not know the query. Other errors are returned unchanged.
func InRegion(err error, region, fallbackQuery string) error {
	typed, ok := usererror.As(err)
	if !ok || typed.Code != usererror.CodeNotFound || strings.TrimSpace(region) == "" {
		return err
	}
	label := i18n.RegionLabel(region)
	query := queryOf(typed.Message)
	if query == "" {
		query = i18n.EchoQuery(strings.TrimSpace(fallbackQuery))
	}
	var message i18n.Message
	switch typed.Message.ID {
	case "music.not_found", "music.not_found_unspecified":
		if query == "" {
			message = i18n.M("music.not_found_unspecified_in_region", i18n.Data{"Region": label})
		} else {
			message = i18n.M("music.not_found_in_region", i18n.Data{"Region": label, "Query": query})
		}
	case "card.not_found", "card.not_found_unspecified":
		if query == "" {
			message = i18n.M("card.not_found_unspecified_in_region", i18n.Data{"Region": label})
		} else {
			message = i18n.M("card.not_found_in_region", i18n.Data{"Region": label, "Query": query})
		}
	case "event.not_found":
		message = i18n.M("event.not_found_in_region_hint", i18n.Data{"Region": label})
	default:
		return err
	}
	return usererror.Wrap(usererror.CodeNotFound, message, typed.Cause)
}

func queryOf(message i18n.Message) string {
	query, _ := message.Data["Query"].(string)
	return query
}

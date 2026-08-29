package ent_test

import (
	"reflect"
	"testing"

	botschema "haruki-cloud/ent/bot/schema"
	censorschema "haruki-cloud/ent/censor/schema"
	chunithmmainschema "haruki-cloud/ent/chunithm/maindb/schema"
	chunithmmusicschema "haruki-cloud/ent/chunithm/music/schema"
	pjskschema "haruki-cloud/ent/pjsk/schema"
	usersschema "haruki-cloud/ent/users/schema"
)

func TestHandwrittenEntSchemasBuildTheirDescriptors(t *testing.T) {
	schemas := []any{
		botschema.CommandLog{},
		botschema.CommandManifest{},
		botschema.DailyRequests{},
		botschema.HourlyRequests{},
		botschema.RequestsRanking{},
		botschema.User{},
		censorschema.ImageModCache{},
		censorschema.NameLog{},
		censorschema.Result{},
		censorschema.ShortBio{},
		chunithmmainschema.ChunithmBinding{},
		chunithmmainschema.ChunithmDefaultServer{},
		chunithmmainschema.ChunithmMusicAlias{},
		chunithmmusicschema.ChunithmChartData{},
		chunithmmusicschema.ChunithmMusic{},
		chunithmmusicschema.ChunithmMusicDifficulty{},
		pjskschema.Alias{},
		pjskschema.AliasAdmin{},
		pjskschema.AliasSubmissionBan{},
		pjskschema.GameAccount{},
		pjskschema.GroupAlias{},
		pjskschema.MysekaiBirthdaySubscription{},
		pjskschema.MysekaiBirthdaySubscriptionEvent{},
		pjskschema.PendingAlias{},
		pjskschema.RejectedAlias{},
		pjskschema.UserBinding{},
		pjskschema.UserDefaultBinding{},
		pjskschema.UserPreference{},
		usersschema.User{},
	}

	for _, schema := range schemas {
		value := reflect.ValueOf(schema)
		called := 0
		for _, methodName := range []string{"Annotations", "Fields", "Edges", "Indexes"} {
			method := value.MethodByName(methodName)
			if !method.IsValid() {
				continue
			}
			results := method.Call(nil)
			if len(results) != 1 {
				t.Fatalf("%T.%s returned %d values", schema, methodName, len(results))
			}
			called++
		}
		if called == 0 {
			t.Fatalf("%T exposes no Ent schema descriptors", schema)
		}
	}
}

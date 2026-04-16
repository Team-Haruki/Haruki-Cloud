package userdata

import (
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestSnapshotFactoryBuildParsesMusicResultsFromUserMusics(t *testing.T) {
	snapshot, err := NewDefaultSnapshotFactory(nil, nil).Build(nil, BuildInput{
		Region: renderregion.CN,
		Source: "toolbox",
		SuiteJSON: []byte(`{
  "now": 1710000000,
  "userGamedata": {"userId": 123456789, "name": "SnapshotUser", "deck": 1, "rank": 100, "coin": 0},
  "userProfile": {"profileImageType": "default", "profileImageId": 0, "word": "", "twitterId": ""},
  "userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1001, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
  "userCards": [{"cardId": 1001, "level": 60, "masterRank": 0, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}],
  "userMusicResults": [],
  "userMusics": [
    {
      "musicId": 101,
      "userMusicDifficultyStatuses": [
        {
          "musicDifficulty": "expert",
          "userMusicResults": [
            {
              "musicId": 101,
              "musicDifficulty": "expert",
              "playResult": "clear",
              "fullComboFlg": false,
              "fullPerfectFlg": false
            }
          ]
        },
        {
          "musicDifficulty": "master",
          "userMusicResults": [
            {
              "playResult": "full_combo",
              "fullComboFlg": true,
              "fullPerfectFlg": false
            }
          ]
        }
      ]
    }
  ],
  "userChallengeLiveSoloResults": [],
  "userChallengeLiveSoloStages": [],
  "userChallengeLiveSoloHighScoreRewards": []
}`),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got := snapshot.GetMusicResult(101, "expert"); got != "clear" {
		t.Fatalf("expected expert result clear, got %q", got)
	}
	if got := snapshot.GetMusicResult(101, "master"); got != "fc" {
		t.Fatalf("expected master result fc, got %q", got)
	}
}

func TestSnapshotFactoryBuildParsesCompactUserMusicResults(t *testing.T) {
	testCases := []struct {
		name          string
		difficultyKey string
	}{
		{name: "cn compact payload", difficultyKey: "musicDifficulty"},
		{name: "tw compact payload", difficultyKey: "musicDifficultyType"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := NewDefaultSnapshotFactory(nil, nil).Build(nil, BuildInput{
				Region: renderregion.CN,
				Source: "toolbox",
				SuiteJSON: []byte(`{
  "now": 1710000000,
  "userGamedata": {"userId": 123456789, "name": "SnapshotUser", "deck": 1, "rank": 100, "coin": 0},
  "userProfile": {"profileImageType": "default", "profileImageId": 0, "word": "", "twitterId": ""},
  "userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1001, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
  "userCards": [{"cardId": 1001, "level": 60, "masterRank": 0, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}],
  "userMusicResults": [],
  "compactUserMusicResults": {
    "__ENUM__": {
      "` + tc.difficultyKey + `": ["easy", "normal", "hard", "expert", "master", "append"],
      "playType": ["solo", "multi"],
      "playResult": ["full_perfect", "full_combo", "clear", "not_clear"]
    },
    "musicId": [201, 202, 203],
    "` + tc.difficultyKey + `": [3, 4, 5],
    "playType": [0, 1, 1],
    "playResult": [2, 1, 0],
    "fullComboFlg": [false, true, true],
    "fullPerfectFlg": [false, false, true]
  },
  "userChallengeLiveSoloResults": [],
  "userChallengeLiveSoloStages": [],
  "userChallengeLiveSoloHighScoreRewards": []
}`),
			})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			if got := snapshot.GetMusicResult(201, "expert"); got != "clear" {
				t.Fatalf("expected expert result clear, got %q", got)
			}
			if got := snapshot.GetMusicResult(202, "master"); got != "fc" {
				t.Fatalf("expected master result fc, got %q", got)
			}
			if got := snapshot.GetMusicResult(203, "append"); got != "ap" {
				t.Fatalf("expected append result ap, got %q", got)
			}
		})
	}
}

package ytmusic

import (
	"context"
	"testing"
)

func TestSearchCardShelf(t *testing.T) {
	client := NewClient("")
	cands, err := client.Search(context.Background(), "Нельсон Вертайсь росою")
	if err != nil {
		t.Fatalf("search err: %v", err)
	}

	if len(cands) == 0 {
		t.Fatalf("expected candidates, got 0")
	}

	top := cands[0]
	if top.VideoID != "Hb0wGzU2BEk" {
		t.Errorf("expected candidate [0] videoId to be Hb0wGzU2BEk, got: %s", top.VideoID)
	}
	if top.Title != "Вертайсь росою" {
		t.Errorf("expected title 'Вертайсь росою', got '%s'", top.Title)
	}
	if len(top.Artists) == 0 || top.Artists[0] != "Нельсон" {
		t.Errorf("expected artist 'Нельсон', got %v", top.Artists)
	}
	if top.DurationMs == 0 {
		t.Errorf("expected non-zero duration, got %d", top.DurationMs)
	}
}

func TestValidateAuth_EmptyCookie(t *testing.T) {
	client := NewClient("")
	_, err := client.ValidateAuth(context.Background())
	if err == nil {
		t.Fatalf("expected error for empty cookie, got nil")
	}
}

func TestValidateAuth_MissingSAPISID(t *testing.T) {
	client := NewClient("VISITOR_INFO1_LIVE=12345; PREF=f1=50000000")
	_, err := client.ValidateAuth(context.Background())
	if err == nil {
		t.Fatalf("expected error for cookie missing SAPISID, got nil")
	}
}

func TestExtractAccountName(t *testing.T) {
	sampleData := map[string]interface{}{
		"activeAccountHeaderRenderer": map[string]interface{}{
			"accountName": map[string]interface{}{
				"runs": []interface{}{
					map[string]interface{}{
						"text": "Music Fan",
					},
				},
			},
			"channelHandle": map[string]interface{}{
				"runs": []interface{}{
					map[string]interface{}{
						"text": "@musicfan",
					},
				},
			},
		},
	}

	name := extractAccountName(sampleData)
	if name != "Music Fan" {
		t.Errorf("expected 'Music Fan', got '%s'", name)
	}
}

func TestExtractContinuationToken(t *testing.T) {
	sample := map[string]interface{}{
		"continuationItemRenderer": map[string]interface{}{
			"continuationEndpoint": map[string]interface{}{
				"continuationCommand": map[string]interface{}{
					"token": "4qmFsgI0EiRWTFBMTUM5S05rSW5jS3RQemdZLTVybWh2ajdmYX",
				},
			},
		},
	}

	token := extractContinuationToken(sample)
	if token != "4qmFsgI0EiRWTFBMTUM5S05rSW5jS3RQemdZLTVybWh2ajdmYX" {
		t.Errorf("expected token '4qmFsgI0EiRWTFBMTUM5S05rSW5jS3RQemdZLTVybWh2ajdmYX', got '%s'", token)
	}
}



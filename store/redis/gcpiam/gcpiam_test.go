package gcpiam_test

import (
	"strings"
	"testing"

	storeredis "github.com/rakunlabs/cache/store/redis"
	"github.com/rakunlabs/cache/store/redis/gcpiam"
)

func TestApplyRejectsStaticCredentials(t *testing.T) {
	for _, cfg := range []storeredis.ConnConfig{{Username: "user"}, {Password: "pass"}} {
		err := gcpiam.Apply(&cfg)
		if err == nil || !strings.Contains(err.Error(), "gcp_iam") {
			t.Fatalf("err = %v, want gcp_iam error", err)
		}
		if cfg.CredentialsProvider != nil {
			t.Fatal("credentials provider set on error")
		}
	}
}

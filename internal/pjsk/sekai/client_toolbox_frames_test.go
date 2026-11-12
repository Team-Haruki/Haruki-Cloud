package sekai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"haruki-cloud/config"
)

func TestSponsorFramePrivateAPIContract(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/private/game-frame/cn/1234" || r.Header.Get("Authorization") != "Bearer frame-test" || r.Header.Get("User-Agent") != "Haruki-Cloud-test" {
			t.Error("incorrect private frame request")
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"frame":{"id":"example","horizontal":{"base":"base.png"},"vertical":{"base":"vertical.png"}}}`))
		}
	}))
	defer srv.Close()
	c := NewToolboxClient(&config.ToolboxConfig{BaseURL: srv.URL, APIToken: "Bearer frame-test", UserAgent: "Haruki-Cloud-test"})
	f, err := c.GetSponsorFrameContext(t.Context(), "cn", "1234")
	if err != nil || f == nil || f.ID != "example" || f.Vertical.Base != "vertical.png" {
		t.Fatalf("frame=%+v err=%v", f, err)
	}
	status = http.StatusNotFound
	if f, err = c.GetSponsorFrameContext(t.Context(), "cn", "1234"); err != nil || f != nil {
		t.Fatal("old backend must remain compatible")
	}
	status = http.StatusForbidden
	if _, err = c.GetSponsorFrameContext(t.Context(), "cn", "1234"); err == nil {
		t.Fatal("unexpected forbidden result accepted")
	}
	if _, err = (*HarukiToolboxClient)(nil).GetSponsorFrameContext(t.Context(), "cn", "1234"); err == nil {
		t.Fatal("nil client accepted")
	}
}

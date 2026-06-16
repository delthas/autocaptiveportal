package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmlpkg "html"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
)

type NetInfo struct {
	SSID      string
	Interface string
	UUID      string

	// Client's DNS is pinned to the Wi-Fi interface's own nameserver, so
	// captive-portal hostnames resolve even when the system resolver is
	// pinned elsewhere (VPN, DoH).
	Client *http.Client
}

type Handler struct {
	Login func(ctx context.Context, info NetInfo) error
	// Metered, if true, sets connection.metered=yes on the NM profile.
	Metered bool
}

var handlers = map[string]Handler{
	"NormandieTrainConnecte": {Metered: true, Login: normandieTrainConnecte},
	"_SNCF_WIFI_INOUI":       {Metered: true, Login: sncfInoui},
	"OUIFI":                  {Metered: true, Login: ouifi},
}

func normandieTrainConnecte(ctx context.Context, info NetInfo) error {
	return sncfActivate(ctx, info, "wifi.normandie.fr", "{}")
}

func sncfInoui(ctx context.Context, info NetInfo) error {
	return sncfActivate(ctx, info, "wifi.sncf", "{}")
}

// sncfActivate POSTs an activation request to the shared VSCT/SNCF portal
// API used by multiple SNCF train Wi-Fi networks.
func sncfActivate(ctx context.Context, info NetInfo, host, body string) error {
	url := "https://" + host + "/router/api/connection/activate/auto"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := info.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// ouifi submits the OUIGO captive portal's "I accept" form. The portal is a
// Next.js app whose form invokes a Server Action; the action ID is hashed
// per build, so we GET the page first and scrape the hidden inputs, then
// POST them back as multipart/form-data. The `1_` prefix on field names is
// added by the React submit handler at runtime, not present in the HTML.
func ouifi(ctx context.Context, info NetInfo) error {
	const portalURL = "https://captif.ouigo.com/portal/en"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, portalURL, nil)
	if err != nil {
		return err
	}
	resp, err := info.Client.Do(req)
	if err != nil {
		return err
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET portal: status %d", resp.StatusCode)
	}

	actionID := extractHiddenInput(page, "$ACTION_1:0")
	actionKey := extractHiddenInput(page, "$ACTION_KEY")
	if actionID == "" || actionKey == "" {
		return errors.New("portal page missing $ACTION fields")
	}

	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	mp.WriteField("1_$ACTION_REF_1", "")
	mp.WriteField("1_$ACTION_1:0", actionID)
	mp.WriteField("1_$ACTION_1:1", "[false]")
	mp.WriteField("1_$ACTION_KEY", actionKey)
	mp.Close()

	req, err = http.NewRequestWithContext(ctx, http.MethodPost, portalURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Accept", "text/x-component")

	resp, err = info.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("activate: status %d", resp.StatusCode)
	}
	return nil
}

// extractHiddenInput finds the HTML-unescaped value of `<input ... name="<name>"
// ... value="..." ...>` in the given HTML. Assumes attribute order
// type/name/value as Next.js renders.
func extractHiddenInput(page []byte, name string) string {
	re := regexp.MustCompile(`<input[^>]*name="` + regexp.QuoteMeta(name) + `"[^>]*value="([^"]*)"`)
	m := re.FindSubmatch(page)
	if m == nil {
		return ""
	}
	return htmlpkg.UnescapeString(string(m[1]))
}

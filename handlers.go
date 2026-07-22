package main

import (
	"context"
	"errors"
	"fmt"
	htmlpkg "html"
	"io"
	"math/rand"
	"net/http"
	"net/url"
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
	"ASK4 WiFi":              {Login: ask4},
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

// ask4 walks the ASK4 guest-signup form: GET the page to obtain the
// PHPSESSID cookie and a per-session CSRF token, then POST the form with
// random first/last names and the token.
func ask4(ctx context.Context, info NetInfo) error {
	const signup = "https://signup.ask4.com/en/guest-signup"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signup, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := info.Client.Do(req)
	if err != nil {
		return err
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET guest-signup: status %d", resp.StatusCode)
	}

	token := extractHiddenInput(page, "personal_details[_token]")
	if token == "" {
		return errors.New("guest-signup: missing _token")
	}

	form := url.Values{
		"personal_details[firstname]": {randAZ(8)},
		"personal_details[surname]":   {randAZ(8)},
		"personal_details[_token]":    {token},
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, signup, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err = info.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("POST guest-signup: status %d", resp.StatusCode)
	}
	return nil
}

// extractHiddenInput finds the HTML-unescaped value of `<input ... name="<name>"
// ... value="..." ...>` in the given HTML.
func extractHiddenInput(page []byte, name string) string {
	re := regexp.MustCompile(`<input[^>]*name="` + regexp.QuoteMeta(name) + `"[^>]*value="([^"]*)"`)
	m := re.FindSubmatch(page)
	if m == nil {
		return ""
	}
	return htmlpkg.UnescapeString(string(m[1]))
}

func randAZ(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

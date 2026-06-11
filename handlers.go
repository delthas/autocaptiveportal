package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

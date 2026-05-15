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
	"NormandieTrainConnecte": {
		Metered: true,
		Login:   normandieTrainConnecte,
	},
}

func normandieTrainConnecte(ctx context.Context, info NetInfo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://wifi.normandie.fr/router/api/connection/activate/auto",
		strings.NewReader("{}"))
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

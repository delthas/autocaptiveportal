package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	log.SetFlags(0)

	if action := os.Getenv("NM_DISPATCHER_ACTION"); action != "" {
		if action != "connectivity-change" || os.Getenv("CONNECTIVITY_STATE") != "PORTAL" {
			return
		}
	}

	info, ok := resolveNet()
	if !ok {
		return
	}

	handler, ok := handlers[info.SSID]
	if !ok {
		log.Printf("no handler for ssid %q", info.SSID)
		return
	}

	info.Client = buildClient(info.Interface)

	hCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	handlerErr := handler.Login(hCtx, info)
	cancel()

	vCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := verify(vCtx); err == nil {
		if handler.Metered {
			setMetered(info)
		}
		succeed(info.SSID)
	} else if handlerErr != nil {
		fail(info.SSID, handlerErr)
	} else {
		fail(info.SSID, err)
	}
}

// Trust the dispatcher env vars only as a coherent set: if CONNECTION_UUID
// describes a Wi-Fi profile, use it whole; otherwise discard env and rebuild
// the (SSID, Interface, UUID) triple from `dev wifi` to avoid mixing two
// connections' data into one NetInfo.
func resolveNet() (NetInfo, bool) {
	if uuid := os.Getenv("CONNECTION_UUID"); uuid != "" {
		iface := os.Getenv("DEVICE_IFACE")
		if iface != "" {
			out, err := exec.Command("nmcli", "-t", "-f", "802-11-wireless.ssid", "connection", "show", uuid).Output()
			if err == nil {
				if fields := nmcliFields(firstLine(out)); len(fields) == 2 && fields[1] != "" {
					return NetInfo{SSID: fields[1], Interface: iface, UUID: uuid}, true
				}
			}
		}
	}
	out, err := exec.Command("nmcli", "-t", "-f", "active,ssid,device", "dev", "wifi").Output()
	if err != nil {
		return NetInfo{}, false
	}
	var info NetInfo
	for _, line := range strings.Split(string(out), "\n") {
		fields := nmcliFields(line)
		if len(fields) >= 3 && fields[0] == "yes" {
			info.SSID = fields[1]
			info.Interface = fields[2]
			break
		}
	}
	if info.SSID == "" || info.Interface == "" {
		return NetInfo{}, false
	}
	if out, err := exec.Command("nmcli", "-t", "-f", "GENERAL.CON-UUID", "dev", "show", info.Interface).Output(); err == nil {
		if fields := nmcliFields(firstLine(out)); len(fields) == 2 {
			info.UUID = fields[1]
		}
	}
	return info, true
}

// nmcliFields splits one line of nmcli's terse output into its fields,
// respecting nmcli's escaping of ':' as '\:' and '\' as '\\' inside values.
func nmcliFields(line string) []string {
	var fields []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			cur.WriteByte(line[i+1])
			i++
			continue
		}
		if line[i] == ':' {
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(line[i])
	}
	fields = append(fields, cur.String())
	return fields
}

func firstLine(b []byte) string {
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func buildClient(iface string) *http.Client {
	dns := ifaceDNS(iface)
	if dns == "" {
		return http.DefaultClient
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			d.Timeout = 2 * time.Second
			return d.DialContext(ctx, network, net.JoinHostPort(dns, "53"))
		},
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, Resolver: resolver}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
}

func setMetered(info NetInfo) {
	if info.UUID == "" {
		return
	}
	_ = exec.Command("nmcli", "connection", "modify", info.UUID, "connection.metered", "yes").Run()
}

func ifaceDNS(iface string) string {
	if iface == "" {
		return ""
	}
	out, err := exec.Command("nmcli", "-t", "-f", "IP4.DNS", "dev", "show", iface).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := nmcliFields(line)
		if len(fields) == 2 && fields[1] != "" {
			return fields[1]
		}
	}
	return ""
}

func verify(ctx context.Context) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var lastErr error
	for i := 0; i < 5; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				if lastErr == nil {
					lastErr = ctx.Err()
				}
				return lastErr
			case <-time.After(time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://detectportal.firefox.com/success.txt", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && string(body) == "success\n" {
			return nil
		}
		lastErr = fmt.Errorf("probe got status %d", resp.StatusCode)
	}
	if lastErr == nil {
		lastErr = errors.New("verification failed")
	}
	return lastErr
}

func succeed(ssid string) {
	log.Printf("connected to %s", ssid)
	notify(fmt.Sprintf("Connected to %s", ssid))
}

func fail(ssid string, err error) {
	log.Printf("failed to connect to %s: %v", ssid, err)
	notify(fmt.Sprintf("Failed to connect to %s: %v", ssid, err))
}

func notify(body string) {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return
	}
	if os.Geteuid() != 0 {
		_ = exec.Command("notify-send", "autocaptiveportal", body).Run()
		return
	}
	out, err := exec.Command("/usr/bin/loginctl", "list-sessions", "--output=json").Output()
	if err != nil {
		return
	}
	var sessions []struct {
		Session string `json:"session"`
		UID     int    `json:"uid"`
	}
	if err := json.Unmarshal(out, &sessions); err != nil {
		return
	}
	for _, s := range sessions {
		props, err := exec.Command("/usr/bin/loginctl", "show-session",
			"-p", "Active", "-p", "Class", "-p", "Type", s.Session).Output()
		if err != nil {
			continue
		}
		p := parseProps(props)
		if p["Active"] != "yes" || p["Class"] != "user" {
			continue
		}
		if p["Type"] != "x11" && p["Type"] != "wayland" {
			continue
		}
		runtimeDir := fmt.Sprintf("/run/user/%d", s.UID)
		cmd := exec.Command("sudo", "-u", fmt.Sprintf("#%d", s.UID),
			"--preserve-env=DBUS_SESSION_BUS_ADDRESS,XDG_RUNTIME_DIR",
			"notify-send", "autocaptiveportal", body)
		cmd.Env = append(os.Environ(),
			"DBUS_SESSION_BUS_ADDRESS=unix:path="+runtimeDir+"/bus",
			"XDG_RUNTIME_DIR="+runtimeDir,
		)
		_ = cmd.Run()
		return
	}
}

func parseProps(b []byte) map[string]string {
	m := make(map[string]string)
	for _, line := range bytes.Split(b, []byte("\n")) {
		k, v, ok := bytes.Cut(line, []byte("="))
		if !ok {
			continue
		}
		m[string(k)] = string(v)
	}
	return m
}

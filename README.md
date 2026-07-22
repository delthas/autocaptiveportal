# autocaptiveportal

Auto-log-in to known Wi-Fi captive portals.

## Why

Captive portals on transport, cafe and hotel Wi-Fi are a recurring nuisance:
you already trust the network, you've already agreed to the terms a hundred
times, and yet every reconnection demands another click. `autocaptiveportal`
lets NetworkManager tell us the moment it sees a portal, then runs the right
HTTP request automatically — no daemon, no clicks.

## Install

```sh
make
sudo make install
```

NetworkManager is required, with its default captive-portal detection
enabled. The installed binary is invoked by NetworkManager's dispatcher; no
extra service or unit is needed.

## Supported portals

- `NormandieTrainConnecte` — SNCF Normandie train Wi-Fi (TER Nomad / TGV
  Inoui Normandie).
- `_SNCF_WIFI_INOUI` — SNCF TGV Inoui train Wi-Fi.
- `*WIFI-AIRPORT` — Paris Aéroport (CDG / Orly) Wi-Fi, operated by Hub One.

## Use

Automatic: nothing to do. When NetworkManager detects a captive portal
(`CONNECTIVITY_STATE=PORTAL`) on a known SSID, the dispatcher runs the
binary.

Manual: `sudo autocaptiveportal` forces an attempt on the currently active
Wi-Fi. Useful for testing a new handler.

## Requirements

- NetworkManager, with default captive-portal detection enabled.
- `nmcli` (ships with NetworkManager).
- Optional: `libnotify` / `notify-send` for desktop toasts on
  success/failure.
- Linux + systemd.

## Adding a portal

1. Connect to the captive Wi-Fi and open your browser's developer tools
   (Network tab).
2. Log in manually. Identify the single request that flips the gate — it's
   usually a `POST` to a vendor-specific endpoint right after the consent
   screen.
3. Add an entry to `handlers.go` keyed by the SSID, issuing that request.
4. Add the SSID to the *Supported portals* list above.
5. Rebuild and reinstall: `make && sudo make install`.

## License

MIT. See [LICENSE](LICENSE).

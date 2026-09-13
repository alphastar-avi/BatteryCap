# Batterycap

A lightweight, native battery charging limiter and hardware power passthrough CLI for Apple Silicon Macs running **macOS 27 Golden Gate** (and macOS 15+ Sequoia).

## Why Batterycap?

* **Native PowerUI / powerd Integration**: Does **not** write to deprecated/guarded SMC keys (`CH0B`, `CH0C`, `CH0I`, `CH0J`). Zero risk of BMS register corruption or kernel panics.
* **True Hardware Passthrough**: When charging reaches the target cap (e.g., 80%), the Apple PMU and TI `bq40z651` gas gauge open the battery charging FETs. The Mac runs 100% on external AC power (USB-C PD VBUS rail) with zero net battery draw.
* **Hardware Fail-Safe**: Power disconnection triggers an analog nanosecond-speed hardware failover to battery power directly at the PMIC level.
* **Zero Background RAM Overhead**: No background daemon, no Electron, no continuous polling. The CLI executes, applies the change via Apple's native Mach XPC service, and exits immediately.
* **Smart Calibration Awareness**: Automatically detects and informs you when macOS runs its occasional BMS recalibration charge (`ChargingUpForGauging`) to maintain gas gauge accuracy.
* **No SIP Disabling or Root Required**: Works completely within standard user permissions.

---

## Commands

| Command | Shorthand / Alternative | Description |
| :--- | :--- | :--- |
| `batterycap status` | `batterycap` | View current battery SoC, power source, charging mode, passthrough state, real battery health (e.g. 87%), and active limit. |
| `batterycap status -j` | `batterycap -j`, `--json` | Output full telemetry & configuration in machine-readable **JSON** format (useful for Sketchybar, Waybar, tmux). |
| `batterycap on [limit]` | `batterycap 80` | Turn on charge limiting and engage hardware passthrough. Defaults to **80%** if omitted. |
| `batterycap set <limit>` | `batterycap <number>` | Set a specific hardware charge limit. Supported values: **`80`**, **`85`**, **`90`**, **`95`**, **`100`**. |
| `batterycap off` | `batterycap disable` | Disable charging limiter (restores normal full charging up to 100%). |
| `batterycap cancel-calibration` | `batterycap uncalibrate`, `resume`, `cancel-override` | Cancel an active 100% calibration charge cycle or temporary session override and lock charging immediately back to your target cap (e.g. 80% or 85%). |
| `batterycap charge-to-full`| `batterycap override` | Temporarily charge to 100% for the current session without altering your saved charge cap. |
| `batterycap limits` | — | Display all native hardware limits supported by this Mac. |
| `batterycap watch` | — | Open a live interactive terminal dashboard updating every 2 seconds (`Ctrl+C` to exit). |
| `batterycap help` | `batterycap -h` | Show the complete command reference and safety guide. |

---

## Building & Installing

```bash
git clone https://github.com/alphastar-avi/BatteryCap.git
cd BatteryCap
make build
make install # copies binary to ~/.local/bin/batterycap
```

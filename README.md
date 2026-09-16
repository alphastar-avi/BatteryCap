# Batterycap

A lightweight, native battery charging limiter and hardware power passthrough CLI for Apple Silicon Macs running **macOS 27 Golden Gate** (and macOS 15+ Sequoia).

---

## Background & Architecture

### Why Legacy SMC Register Approaches No Longer Work
Historically, charge limiter utilities on macOS managed battery charge thresholds by writing directly to legacy Apple SMC registers (primarily `CH0B`, `CH0C`, or `CH0I`) to inhibit battery charging.

On modern macOS releases (macOS 15+ Sequoia and macOS 27 Golden Gate):
* **Registers are Deprecated/Removed**: The legacy keys (`CH0B`, `CH0C`, `CH0I`) are no longer populated or active in Apple's SMC tree.
* **Kernel SIP Restrictions**: The modern charging control register (`CHIB`) is strictly guarded by kernel-level System Integrity Protection (SIP) entitlements (`com.apple.private.applesmc.user-client-access`). Attempting to write directly to the SMC client fails with `kIOReturnNotPrivileged` (`0xe00002c1`), even when running as `root` via `sudo`.

### The Native PowerUI / powerd Approach
Rather than attempting to bypass SIP, patch kernel extensions, or force unauthorized SMC writes, **Batterycap** interfaces directly with macOS's native battery subsystem via `PowerUI.framework` and `powerd` Mach XPC:

* **Native Mach XPC Interface**: Directly communicates with Apple's `com.apple.powerui.smartChargeManager` XPC service in user-space.
* **True Hardware Passthrough**: Once the battery reaches the target cap, the Power Management Unit (PMU) disengages charging circuitry (`PMUConfigured = 0`, `NotChargingReason = 0x01000000`). The Mac runs 100% on external AC power from the USB-C rail with 0 net battery draw.
* **Smart Calibration Management**: Detects when macOS initiates periodic gas gauge recalibration charges (`ChargingUpForGauging`) and manages temporary overrides (`batterycap uncalibrate`). Accurately reports whether a 100% session override was user-initiated or enforced by the PMU for gas gauge impedance tracking, keeping your target limit armed for automatic passthrough resumption.
* **Zero Persistent Overhead**: Operates without background daemons, helper processes, or persistent RAM usage. Commands execute via Mach XPC and exit immediately.

### Supported Charge Limit Range (80% – 100%)

* **Why Limits Below 80% (55%, 65%, 75%) Are Restricted**:
  In `PowerUI.framework`, `PowerUISmartChargeManager` compiles the limit bounds check directly into ARM64 machine instructions (`(limit - 80) <= 20`). Any value below 80 is rejected by the OS as out of bounds.
  Apple enforces this 80% floor to guarantee brownout voltage stability under load: during transient CPU/GPU power spikes exceeding charger wattage, the PMU relies on hybrid battery supplement power, which requires cell voltage to remain at or above the 80% operating plateau. Direct kernel-level writes to bypass this floor require private SIP entitlements.
* **1% Granularity (80% – 100%)**:
  While macOS UI menus only show 5% presets, the underlying PMU controller accepts any individual integer from **80% to 100%** (e.g. `80%`, `82%`, `85%`, `90%`).

---

## Commands

| Command | Shorthand / Alternative | Description |
| :--- | :--- | :--- |
| `batterycap status` | `batterycap` | View current battery SoC, power source, charging mode, passthrough state, real battery health (e.g. 87%), and active limit. |
| `batterycap status -j` | `batterycap -j`, `--json` | Output full telemetry & configuration in machine-readable **JSON** format (useful for Sketchybar, Waybar, tmux). |
| `batterycap on [limit]` | `batterycap 80`, `batterycap 82` | Turn on charge limiting and engage hardware passthrough (80% – 100%). Defaults to **80%** if omitted. |
| `batterycap set <limit>` | `batterycap <number>` | Set a specific hardware charge limit with **1% granularity** (e.g. `batterycap 82`, `batterycap set 87`). Supported range: **`80`** to **`100`**. |
| `batterycap off` | `batterycap disable` | Disable charging limiter (restores normal full charging up to 100%). |
| `batterycap cancel-calibration` | `batterycap uncalibrate`, `resume`, `cancel-override` | Cancel a temporary session override (`charge-to-full`), or inspect and re-arm your target cap (e.g. 80%) during periodic macOS gas gauge recalibration. |
| `batterycap charge-to-full`| `batterycap override` | Temporarily charge to 100% for the current session without altering your saved charge cap. |
| `batterycap limits` | — | Display supported hardware charge limit range (80% – 100%) and native presets. |
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

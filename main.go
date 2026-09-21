package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework IOKit
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ANSI Color Codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
	colorWhite  = "\033[97m"
)

type CellInfo struct {
	BankID    int     `json:"bank_id"`
	VoltageMv int     `json:"voltage_mv"`
	VoltageV  float64 `json:"voltage_v"`
	QmaxMah   int     `json:"qmax_mah"`
}

type HardwareInfo struct {
	DeviceName               string     `json:"device_name"`
	Condition                string     `json:"condition"`
	CycleCount               int        `json:"cycle_count"`
	CurrentCapacity          int        `json:"current_capacity_percent"`
	HealthMaxPercent         int        `json:"health_max_capacity_percent"`
	NominalCapacity          int        `json:"nominal_capacity_mah"`
	DesignCapacity           int        `json:"design_capacity_mah"`
	FullChargeCapacity       int        `json:"full_charge_capacity_mah"`
	RemainingCapacity        int        `json:"remaining_capacity_mah"`
	AvgTimeToFullMinutes     int        `json:"avg_time_to_full_minutes"`
	LastCalibrationTimestamp int64      `json:"last_calibration_timestamp"`
	FullyCharged             bool       `json:"fully_charged"`
	VoltageMv                int        `json:"voltage_mv"`
	VoltageV                 float64    `json:"voltage_v"`
	AmperageMa               int        `json:"amperage_ma"`
	IsCharging               bool       `json:"is_charging"`
	ExternalConnected        bool       `json:"external_connected"`
	IsPassthrough            bool       `json:"is_passthrough"`
	UIState                  int        `json:"ui_state"`
	Cells                    []CellInfo `json:"cells"`
	CellCount                int        `json:"cell_count"`
}

type MCLInfo struct {
	Supported       bool  `json:"supported"`
	Enabled         bool  `json:"enabled"`
	Limit           int   `json:"limit_percent"`
	AvailableLimits []int `json:"available_limits"`
	UIState         int   `json:"ui_state"`
}

type CalibrationInfo struct {
	IsCalibrated             bool       `json:"is_calibrated"`
	InProgress               bool       `json:"in_progress"`
	LastCalibrationTimestamp int64      `json:"last_calibration_timestamp"`
	LastCalibrationDate      string     `json:"last_calibration_date"`
	TimeSinceCalibration     string     `json:"time_since_calibration"`
	EstimatedNextTimestamp   int64      `json:"estimated_next_timestamp"`
	EstimatedNextDate        string     `json:"estimated_next_date"`
	EstimatedTimeRemaining   string     `json:"estimated_time_remaining"`
	Algorithm                string     `json:"algorithm"`
	Controller               string     `json:"controller"`
	FullChargeCapacityMah    int        `json:"full_charge_capacity_mah"`
	RemainingCapacityMah     int        `json:"remaining_capacity_mah"`
	CapacityRemainingToFull  int        `json:"capacity_remaining_to_full_mah"`
	MinutesRemainingToFull   int        `json:"minutes_remaining_to_full"`
	DesignCapacityMah        int        `json:"design_capacity_mah"`
	ActiveChargeLimit        int        `json:"active_charge_limit_percent"`
	Cells                    []CellInfo `json:"cells"`
}

type BatteryReport struct {
	Hardware    HardwareInfo    `json:"hardware"`
	Limiter     MCLInfo         `json:"limiter"`
	Calibration CalibrationInfo `json:"calibration"`
}

func getHardwareInfo() (HardwareInfo, error) {
	var cInfo C.BatteryHardwareInfo
	res := C.BatteryGetHardwareInfo(&cInfo)
	if res != 0 {
		return HardwareInfo{}, fmt.Errorf("failed to query AppleSmartBattery from IOKit")
	}

	devName := C.GoString(&cInfo.deviceName[0])
	cond := C.GoString(&cInfo.healthCondition[0])
	voltMv := int(cInfo.voltageMv)

	var cells []CellInfo
	cellCnt := int(cInfo.cellCount)
	for i := 0; i < cellCnt && i < 4; i++ {
		vMv := int(cInfo.cells[i].voltageMv)
		cells = append(cells, CellInfo{
			BankID:    int(cInfo.cells[i].bankId),
			VoltageMv: vMv,
			VoltageV:  float64(vMv) / 1000.0,
			QmaxMah:   int(cInfo.cells[i].qmaxMah),
		})
	}

	return HardwareInfo{
		DeviceName:               devName,
		Condition:                cond,
		CycleCount:               int(cInfo.cycleCount),
		CurrentCapacity:          int(cInfo.currentCapacity),
		HealthMaxPercent:         int(cInfo.healthMaxPercent),
		NominalCapacity:          int(cInfo.nominalCapacity),
		DesignCapacity:           int(cInfo.designCapacity),
		FullChargeCapacity:       int(cInfo.fullChargeCapacity),
		RemainingCapacity:        int(cInfo.remainingCapacity),
		AvgTimeToFullMinutes:     int(cInfo.avgTimeToFullMinutes),
		LastCalibrationTimestamp: int64(cInfo.lastCalibrationTimestamp),
		FullyCharged:             bool(cInfo.fullyCharged),
		VoltageMv:                voltMv,
		VoltageV:                 float64(voltMv) / 1000.0,
		AmperageMa:               int(cInfo.amperageMa),
		IsCharging:               bool(cInfo.isCharging),
		ExternalConnected:        bool(cInfo.externalConnected),
		IsPassthrough:            bool(cInfo.isPassthrough),
		UIState:                  int(cInfo.uiState),
		Cells:                    cells,
		CellCount:                cellCnt,
	}, nil
}

func getMCLInfo() (MCLInfo, error) {
	var cEnabled, cLimit, cSupported, cUIState C.int
	cLimits := make([]C.int, 16)
	cCount := C.int(len(cLimits))

	res := C.BatteryGetMCLStatus(&cEnabled, &cLimit, &cSupported, &cLimits[0], &cCount, &cUIState)
	if res != 0 {
		return MCLInfo{}, fmt.Errorf("failed to query PowerUI SmartChargeClient")
	}

	avail := make([]int, int(cCount))
	for i := 0; i < int(cCount); i++ {
		avail[i] = int(cLimits[i])
	}

	return MCLInfo{
		Supported:       cSupported != 0,
		Enabled:         cEnabled != 0,
		Limit:           int(cLimit),
		AvailableLimits: avail,
		UIState:         int(cUIState),
	}, nil
}

func isValidLimit(limit int) bool {
	return limit >= 80 && limit <= 100
}

func setMCLLimit(limit int) error {
	if !isValidLimit(limit) {
		return fmt.Errorf("limit %d%% is out of bounds (supported native range: 80%% – 100%%).\n    macOS firmware enforces an 80%% minimum floor to guarantee peak load voltage stability", limit)
	}

	var errBuf [256]C.char
	res := C.BatterySetMCLLimit(C.uchar(limit), &errBuf[0], C.int(len(errBuf)))
	if res != 0 {
		errMsg := C.GoString(&errBuf[0])
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

func disableMCL() error {
	var errBuf [256]C.char
	res := C.BatteryDisableMCL(&errBuf[0], C.int(len(errBuf)))
	if res != 0 {
		errMsg := C.GoString(&errBuf[0])
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

func overrideFull() error {
	var errBuf [256]C.char
	res := C.BatteryOverrideFull(&errBuf[0], C.int(len(errBuf)))
	if res != 0 {
		errMsg := C.GoString(&errBuf[0])
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

func cancelCalibration(limit int) (int, error) {
	var errBuf [256]C.char
	res := C.BatteryCancelCalibration(C.uchar(limit), &errBuf[0], C.int(len(errBuf)))
	if res < 0 {
		errMsg := C.GoString(&errBuf[0])
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return -1, fmt.Errorf("%s", errMsg)
	}
	return int(res), nil
}

type LimiterConfig struct {
	SavedLimit int `json:"saved_limit"`
}

func configFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "batterycap", "config.json")
}

func loadSavedLimit() int {
	p := configFilePath()
	if p == "" {
		return 0
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	var cfg LimiterConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0
	}
	if cfg.SavedLimit >= 80 && cfg.SavedLimit < 100 {
		return cfg.SavedLimit
	}
	return 0
}

func saveSavedLimit(limit int) {
	if limit < 80 || limit >= 100 {
		return
	}
	p := configFilePath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	data, _ := json.MarshalIndent(LimiterConfig{SavedLimit: limit}, "", "  ")
	_ = os.WriteFile(p, data, 0644)
}

func progressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := (percent * width) / 100
	empty := width - filled

	barColor := colorGreen
	if percent <= 20 {
		barColor = colorRed
	} else if percent <= 50 {
		barColor = colorYellow
	}

	return fmt.Sprintf("%s[%s%s%s%s]",
		colorGray,
		barColor,
		strings.Repeat("█", filled),
		colorGray,
		strings.Repeat("░", empty)+colorGray,
	) + colorReset
}

func computeCalibrationInfo(hw HardwareInfo, mcl MCLInfo) CalibrationInfo {
	// Calibration is only actively charging if UIState is 18 AND battery is actively charging or not fully charged
	isCalibCharging := (hw.UIState == 18) && (hw.IsCharging || (hw.CurrentCapacity < 99 && !hw.FullyCharged))

	remMah := 0
	if hw.FullChargeCapacity > hw.RemainingCapacity {
		remMah = hw.FullChargeCapacity - hw.RemainingCapacity
	}

	info := CalibrationInfo{
		InProgress:              isCalibCharging,
		Algorithm:               "Texas Instruments Impedance Track (IT)",
		Controller:              hw.DeviceName,
		FullChargeCapacityMah:   hw.FullChargeCapacity,
		RemainingCapacityMah:    hw.RemainingCapacity,
		CapacityRemainingToFull: remMah,
		MinutesRemainingToFull:  hw.AvgTimeToFullMinutes,
		DesignCapacityMah:       hw.DesignCapacity,
		ActiveChargeLimit:       mcl.Limit,
		Cells:                   hw.Cells,
	}

	if hw.LastCalibrationTimestamp > 0 {
		info.IsCalibrated = !isCalibCharging
		info.LastCalibrationTimestamp = hw.LastCalibrationTimestamp
		lastDate := time.Unix(hw.LastCalibrationTimestamp, 0)
		info.LastCalibrationDate = lastDate.Format("Mon Jan 02 15:04:05 MST 2006")

		since := time.Since(lastDate)
		if since < time.Minute {
			info.TimeSinceCalibration = "just now"
		} else if since < time.Hour {
			info.TimeSinceCalibration = fmt.Sprintf("%d minutes ago", int(since.Minutes()))
		} else if since < 24*time.Hour {
			info.TimeSinceCalibration = fmt.Sprintf("%.1f hours ago", since.Hours())
		} else {
			info.TimeSinceCalibration = fmt.Sprintf("%.1f days ago", since.Hours()/24.0)
		}

		// Rolling 72-hour window under continuous AC hold at MCL
		nextDate := lastDate.Add(72 * time.Hour)
		info.EstimatedNextTimestamp = nextDate.Unix()
		info.EstimatedNextDate = nextDate.Format("Mon Jan 02 15:04:05 MST 2006")

		remaining := time.Until(nextDate)
		if info.InProgress {
			if hw.AvgTimeToFullMinutes > 0 && hw.AvgTimeToFullMinutes < 60000 {
				info.EstimatedTimeRemaining = fmt.Sprintf("~%d min remaining until 100%% reference", hw.AvgTimeToFullMinutes)
			} else {
				info.EstimatedTimeRemaining = "In progress (charging towards 100%)"
			}
		} else if remaining > 0 {
			if remaining > 24*time.Hour {
				days := int(remaining.Hours()) / 24
				hours := int(remaining.Hours()) % 24
				info.EstimatedTimeRemaining = fmt.Sprintf("~%d days %d hours remaining", days, hours)
			} else {
				info.EstimatedTimeRemaining = fmt.Sprintf("~%.1f hours remaining", remaining.Hours())
			}
		} else {
			info.EstimatedTimeRemaining = "Due soon under continuous AC hold"
		}
	} else {
		info.IsCalibrated = false
		info.LastCalibrationDate = "Unknown"
		info.TimeSinceCalibration = "Unknown"
		info.EstimatedNextDate = "Unknown"
		info.EstimatedTimeRemaining = "Unknown"
	}

	return info
}

func printStatus(jsonOutput bool) {
	hw, errHw := getHardwareInfo()
	if errHw != nil {
		fmt.Fprintf(os.Stderr, "%s[!] Error retrieving hardware telemetry: %v%s\n", colorRed, errHw, colorReset)
		os.Exit(1)
	}

	mcl, errMcl := getMCLInfo()
	if errMcl != nil {
		fmt.Fprintf(os.Stderr, "%s[!] Error retrieving limiter configuration: %v%s\n", colorRed, errMcl, colorReset)
		os.Exit(1)
	}

	calib := computeCalibrationInfo(hw, mcl)

	if jsonOutput {
		report := BatteryReport{Hardware: hw, Limiter: mcl, Calibration: calib}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("\n%s%sBatterycap%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s──────────────────────────────────────────────────────────────────%s\n", colorGray, colorReset)

	// Hardware Controller & Source
	fmt.Printf("  %sHardware Controller:%s     %s\n", colorBold, colorReset, hw.DeviceName)

	powerSourceStr := fmt.Sprintf("%sAC Power Adapter (Connected)%s", colorGreen, colorReset)
	if !hw.ExternalConnected {
		powerSourceStr = fmt.Sprintf("%sBattery Power (Discharging)%s", colorYellow, colorReset)
	}
	fmt.Printf("  %sPower Source:%s            %s\n", colorBold, colorReset, powerSourceStr)

	// Battery Level & Health
	bar := progressBar(hw.CurrentCapacity, 15)
	currentStr := fmt.Sprintf("%d mA", hw.AmperageMa)
	if hw.AmperageMa > 0 {
		currentStr = fmt.Sprintf("+%d mA (inflow)", hw.AmperageMa)
	} else if hw.AmperageMa < 0 {
		currentStr = fmt.Sprintf("%d mA (drain)", hw.AmperageMa)
	} else {
		currentStr = "0 mA (passthrough idle)"
	}

	fmt.Printf("  %sBattery SoC Level:%s       %s %s%d%%%s  (%.2f V, %s)\n",
		colorBold, colorReset, bar, colorBold, hw.CurrentCapacity, colorReset, hw.VoltageV, currentStr)
	fmt.Printf("  %sBattery Health / Max:%s    %s%d%%%s  (Condition: %s | %d cycles)\n",
		colorBold, colorReset, colorBold, hw.HealthMaxPercent, colorReset, hw.Condition, hw.CycleCount)
	if hw.NominalCapacity > 0 && hw.DesignCapacity > 0 {
		fmt.Printf("  %sCapacity Telemetry:%s      %d mAh nominal / %d mAh design\n",
			colorGray, colorReset, hw.NominalCapacity, hw.DesignCapacity)
	}

	// Gas Gauge Calibration Telemetry
	if calib.InProgress {
		timeRemStr := ""
		if hw.AvgTimeToFullMinutes > 0 && hw.AvgTimeToFullMinutes < 60000 {
			timeRemStr = fmt.Sprintf(" (~%d min to full)", hw.AvgTimeToFullMinutes)
		}
		capRemStr := ""
		if calib.CapacityRemainingToFull > 0 {
			capRemStr = fmt.Sprintf(" | %d mAh to full", calib.CapacityRemainingToFull)
		}
		fmt.Printf("  %sGas Gauge Status:%s        %s%sRecalibration In Progress (Targeting 100%%%s%s)%s\n",
			colorBold, colorReset, colorBold, colorYellow, timeRemStr, capRemStr, colorReset)
	} else if calib.IsCalibrated {
		if hw.UIState == 18 && !hw.IsCharging && hw.CurrentCapacity >= 99 {
			fmt.Printf("  %sGas Gauge Status:%s        %sCalibrated%s  (100%% Reference Settled | Passthrough Engaged)\n",
				colorBold, colorReset, colorGreen, colorReset)
		} else {
			fmt.Printf("  %sGas Gauge Status:%s        %sCalibrated%s  (Last: %s | Next: %s)\n",
				colorBold, colorReset, colorGreen, colorReset, calib.TimeSinceCalibration, calib.EstimatedTimeRemaining)
		}
	}

	// 3-Cell Battery Pack Telemetry
	if len(hw.Cells) > 0 {
		var cellStrs []string
		for i, c := range hw.Cells {
			cellStrs = append(cellStrs, fmt.Sprintf("C%d: %.2fV (%d mAh)", i+1, c.VoltageV, c.QmaxMah))
		}
		fmt.Printf("  %sCell Architecture:%s       %d Cells — %s\n",
			colorGray, colorReset, len(hw.Cells), strings.Join(cellStrs, " | "))
	}

	// Charging State & Passthrough
	fmt.Printf("%s──────────────────────────────────────────────────────────────────%s\n", colorGray, colorReset)
	if hw.ExternalConnected {
		if hw.UIState == 18 {
			if !hw.IsCharging && hw.CurrentCapacity >= 99 {
				fmt.Printf("  %sCharging Mode:%s           %s%sCalibration Complete — Hardware Passthrough Engaged%s\n",
					colorBold, colorReset, colorBold, colorGreen, colorReset)
				fmt.Printf("  %sPassthrough Telemetry:%s   Battery reached 100%%. Passthrough active (0 mA). %s%d%% limit will auto-resume%s on next cycle.\n",
					colorGray, colorReset, colorBold, mcl.Limit, colorReset)
			} else {
				// ChargingUpForGauging: Apple's periodic battery calibration
				fmt.Printf("  %sCharging Mode:%s           %s%sBattery Calibration Active%s (Target: 100%% temporary)\n",
					colorBold, colorReset, colorBold, colorYellow, colorReset)
				timeNote := ""
				if hw.AvgTimeToFullMinutes > 0 && hw.AvgTimeToFullMinutes < 60000 {
					timeNote = fmt.Sprintf(" (~%d min remaining)", hw.AvgTimeToFullMinutes)
				}
				fmt.Printf("  %sCalibration Note:%s        macOS powerd is running an occasional calibration charge to\n",
					colorYellow, colorReset)
				fmt.Printf("                           maintain gas gauge accuracy%s. %s%d%% limit will auto-resume%s after.\n",
					timeNote, colorBold, mcl.Limit, colorReset)
			}
		} else if hw.IsCharging {
			fmt.Printf("  %sCharging Mode:%s           %sCharging Active%s (Target Cap: %d%%)\n",
				colorBold, colorReset, colorYellow, colorReset, mcl.Limit)
		} else {
			fmt.Printf("  %sCharging Mode:%s           %s%sHardware Passthrough Engaged%s (Inhibited at cap)\n",
				colorBold, colorReset, colorBold, colorGreen, colorReset)
			fmt.Printf("  %sPassthrough Telemetry:%s   Running 100%% on AC VBUS rail. Zero net battery current.\n",
				colorGray, colorReset)
		}
	} else {
		fmt.Printf("  %sCharging Mode:%s           %sOn Battery%s (Instant hardware failover active)\n",
			colorBold, colorReset, colorYellow, colorReset)
	}

	// Limiter Configuration
	mclStatusStr := fmt.Sprintf("%sENABLED%s", colorGreen, colorReset)
	if !mcl.Enabled {
		mclStatusStr = fmt.Sprintf("%sDISABLED (Charging to 100%%)%s", colorRed, colorReset)
	}
	fmt.Printf("  %sManual Charge Limit (MCL):%s %s\n", colorBold, colorReset, mclStatusStr)
	fmt.Printf("  %sActive Charge Limit:%s      %s%d%%%s\n", colorBold, colorReset, colorBold, mcl.Limit, colorReset)

	fmt.Printf("  %sAvailable Native Limits:%s  80%% – 100%% (1%% increments)\n", colorBold, colorReset)

	presetsStr := "[ 80%, 85%, 90%, 95%, 100% ]"
	if len(mcl.AvailableLimits) > 0 {
		var parts []string
		for _, l := range mcl.AvailableLimits {
			parts = append(parts, fmt.Sprintf("%d%%", l))
		}
		presetsStr = fmt.Sprintf("[ %s ]", strings.Join(parts, ", "))
	}
	fmt.Printf("  %sSystem Presets:%s           %s\n", colorBold, colorReset, presetsStr)

	fmt.Printf("%s──────────────────────────────────────────────────────────────────%s\n", colorGray, colorReset)
	fmt.Printf("  %sRuntime Architecture:%s    Apple PowerUI / powerd Mach XPC\n\n", colorGray, colorReset)
}

func printCalibrationInfo(jsonOutput bool) {
	hw, errHw := getHardwareInfo()
	if errHw != nil {
		fmt.Fprintf(os.Stderr, "%s[!] Error retrieving hardware telemetry: %v%s\n", colorRed, errHw, colorReset)
		os.Exit(1)
	}

	mcl, _ := getMCLInfo()
	calib := computeCalibrationInfo(hw, mcl)

	if jsonOutput {
		data, _ := json.MarshalIndent(calib, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("\n%s%sBatterycap Gas Gauge & Calibration Telemetry%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s──────────────────────────────────────────────────────────────────%s\n", colorGray, colorReset)

	statusStr := fmt.Sprintf("%s%sCalibrated (Optimal Gas Gauge Accuracy)%s", colorBold, colorGreen, colorReset)
	if calib.InProgress {
		timeStr := ""
		if hw.AvgTimeToFullMinutes > 0 && hw.AvgTimeToFullMinutes < 60000 {
			timeStr = fmt.Sprintf(" — ~%d min remaining", hw.AvgTimeToFullMinutes)
		}
		statusStr = fmt.Sprintf("%s%sRecalibration In Progress (Targeting 100%%%s)%s", colorBold, colorYellow, timeStr, colorReset)
	} else if hw.UIState == 18 && !hw.IsCharging && hw.CurrentCapacity >= 99 {
		statusStr = fmt.Sprintf("%s%sCalibrated (100%% Target Reached — Passthrough Active)%s", colorBold, colorGreen, colorReset)
	} else if !calib.IsCalibrated {
		statusStr = fmt.Sprintf("%s%sCalibration Needed / Unknown%s", colorBold, colorYellow, colorReset)
	}
	fmt.Printf("  %sGas Gauge Status:%s        %s\n", colorBold, colorReset, statusStr)

	if calib.InProgress {
		if hw.AvgTimeToFullMinutes > 0 && hw.AvgTimeToFullMinutes < 60000 {
			fmt.Printf("  %sTime Remaining to 100%%:%s  ~%d minutes\n", colorBold, colorReset, hw.AvgTimeToFullMinutes)
		}
		if calib.CapacityRemainingToFull > 0 {
			fmt.Printf("  %sCapacity Remaining:%s      %d mAh to reach full target (%d / %d mAh)\n",
				colorBold, colorReset, calib.CapacityRemainingToFull, hw.RemainingCapacity, hw.FullChargeCapacity)
		}
	} else if calib.LastCalibrationTimestamp > 0 {
		fmt.Printf("  %sLast Full Calibration:%s   %s (%s)\n",
			colorBold, colorReset, calib.LastCalibrationDate, calib.TimeSinceCalibration)
		fmt.Printf("  %sEstimated Next Window:%s  ~3 days continuous AC hold (%s)\n",
			colorBold, colorReset, calib.EstimatedNextDate)
		fmt.Printf("  %sEstimated Remaining:%s    %s\n",
			colorBold, colorReset, calib.EstimatedTimeRemaining)
	}

	fmt.Printf("%s──────────────────────────────────────────────────────────────────%s\n", colorGray, colorReset)
	fmt.Printf("  %sHardware Controller:%s     %s\n", colorBold, colorReset, calib.Controller)
	fmt.Printf("  %sAlgorithm Architecture:%s  %s\n", colorBold, colorReset, calib.Algorithm)
	fmt.Printf("  %sCalibrated Full Cap:%s     %d mAh nominal / %d mAh design\n",
		colorBold, colorReset, hw.NominalCapacity, calib.DesignCapacityMah)
	if calib.FullChargeCapacityMah > 0 {
		fmt.Printf("  %sTrue Full Charge Cap:%s    %d mAh (FCC at 100%% reference point)\n",
			colorGray, colorReset, calib.FullChargeCapacityMah)
	}

	if len(hw.Cells) > 0 {
		fmt.Printf("  %s3-Cell Battery Pack:%s\n", colorBold, colorReset)
		for i, c := range hw.Cells {
			fmt.Printf("    • Cell %d (Bank %d):       %.2f V (%d mV) | Qmax: %d mAh\n",
				i+1, c.BankID, c.VoltageV, c.VoltageMv, c.QmaxMah)
		}
	}

	limitStr := fmt.Sprintf("%d%%", calib.ActiveChargeLimit)
	if !mcl.Enabled {
		limitStr = "100% (Limiter Disabled)"
	}
	fmt.Printf("  %sActive Charge Limit:%s     %s (Hardware Passthrough Armed)\n",
		colorBold, colorReset, limitStr)

	fmt.Printf("%s──────────────────────────────────────────────────────────────────%s\n", colorGray, colorReset)
	fmt.Printf("  %sCalibration Context:%s\n", colorGray, colorReset)
	fmt.Printf("    Apple Silicon's PMU calculates battery capacity and health via cell impedance\n")
	fmt.Printf("    tracking. Holding the battery continuously at 80%% causes DOD0 (Depth of Discharge 0)\n")
	fmt.Printf("    reference points to drift. macOS powerd enforces a 100%% calibration top-off every ~72h\n")
	fmt.Printf("    under uninterrupted AC power. Normal on-battery discharge cycles refresh impedance tracking\n")
	fmt.Printf("    naturally, postponing or eliminating the need for periodic calibration charges.\n\n")
}

func runWatchMode() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	fmt.Print("\033[2J")
	for {
		select {
		case <-c:
			fmt.Printf("\n%sExiting watch mode.%s\n", colorGray, colorReset)
			return
		default:
			fmt.Print("\033[H")
			printStatus(false)
			fmt.Printf("  %s[Live Monitor — Press Ctrl+C to exit. Updates every 2s]%s\n", colorGray, colorReset)
			time.Sleep(2 * time.Second)
		}
	}
}

func printHelp() {
	prog := "batterycap"
	if len(os.Args) > 0 {
		prog = os.Args[0]
	}

	fmt.Printf(`
%s%sBatterycap — Native Battery Limiter & Passthrough CLI%s

%sUSAGE:%s
  %s [command] [options]
  %s <limit_percent>

%sAVAILABLE COMMANDS:%s
  %sstatus%s           Display comprehensive battery, charging mode, and hardware passthrough status.
                   Options: %s-j%s, %s--json%s for machine-readable JSON output.

  %son [limit]%s        Enable native charge limiter and hardware passthrough.
                   Defaults to 80%% if limit argument is omitted.
                   Example: %s on 80

  %sset <limit>%s       Set active charge limit percentage directly.
                   Supported range: 80 - 100% (e.g. 80, 82, 85, 90).
                   Example: %s set 85

  %s<number>%s          Shortcut to set and engage a limit directly.
                   Example: %s 80

  %soff%s, %sdisable%s     Disable native charge limit (restores default full charging to 100%%).

  %scancel-calibration%s   Cancel active 100%% calibration or session override; lock to target cap.
  %suncalibrate%s          Aliases: stop-calibration, uncalibrate, resume, cancel-override.
                       Example: %s uncalibrate

  %scalibration%s          Inspect gas gauge calibration status, last calibration timestamp,
  %scalib%s                and estimated next recalibration window.
                       Example: %s calibration

  %scharge-to-full%s       Temporarily override limit to 100%% for the current session without
  %soverride%s             modifying your saved charge threshold.

  %slimits%s               List all hardware charge limit thresholds supported by this Mac.

  %swatch%s                Live updating dashboard monitoring battery SoC, current, and passthrough.

  %shelp%s, %s-h%s, %s--help%s     Show this command reference.

%sSAFETY & ARCHITECTURE:%s
  • Native Apple Silicon: Leverages macOS PowerUI.framework and powerd Mach XPC.
  • Zero SMC writes: Eliminates obsolete register corruption (CH0B/CH0C/CH0I/CH0J).
  • Hardware Passthrough: Disables battery charging FETs; powers directly from USB-C PD VBUS.
  • Hardware Fail-Safe: Instantaneous analog failover to battery if power cord is detached.
  • Zero RAM Footprint: 0 MB persistent memory; no continuous background daemons needed.
  • Safe Calibration: Seamlessly respects Apple's occasional BMS recalibration charges.

`,
		colorBold, colorCyan, colorReset,
		colorBold, colorReset,
		prog, prog,
		colorBold, colorReset,
		colorBold, colorReset, colorGreen, colorReset, colorGreen, colorReset,
		colorBold, colorReset, prog,
		colorBold, colorReset, prog,
		colorBold, colorReset, prog,
		colorBold, colorReset, colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset, prog,
		colorBold, colorReset,
		colorBold, colorReset, prog,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset, colorBold, colorReset, colorBold, colorReset,
		colorBold, colorReset,
	)
}

func main() {
	if len(os.Args) <= 1 {
		printStatus(false)
		return
	}

	cmd := strings.ToLower(os.Args[1])

	switch cmd {
	case "status":
		jsonOut := false
		for _, arg := range os.Args[2:] {
			if arg == "--json" || arg == "-j" {
				jsonOut = true
			}
		}
		printStatus(jsonOut)

	case "--json", "-j":
		printStatus(true)

	case "calibration", "calib", "gauge", "calibration-status", "calibration-info":
		jsonOut := false
		for _, arg := range os.Args[2:] {
			if arg == "--json" || arg == "-j" {
				jsonOut = true
			}
		}
		printCalibrationInfo(jsonOut)

	case "on", "enable":
		targetLimit := 80
		if saved := loadSavedLimit(); saved >= 80 && saved < 100 {
			targetLimit = saved
		}
		if len(os.Args) >= 3 {
			n, err := strconv.Atoi(os.Args[2])
			if err != nil || n < 80 || n > 100 {
				fmt.Fprintf(os.Stderr, "%s[!] Invalid limit '%s'. Supported range: 80 - 100%%%s\n", colorRed, os.Args[2], colorReset)
				os.Exit(1)
			}
			targetLimit = n
		}
		if err := setMCLLimit(targetLimit); err != nil {
			fmt.Fprintf(os.Stderr, "%s[!] Failed to enable limiter: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		if targetLimit < 100 {
			saveSavedLimit(targetLimit)
		}
		fmt.Printf("%s[✓] Charge limit enabled and set to %d%%.%s\n", colorGreen, targetLimit, colorReset)
		fmt.Printf("%s    macOS powerd will cap charging at %d%% and engage hardware passthrough.%s\n", colorGray, targetLimit, colorReset)

	case "set":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "%s[!] Missing target limit value. Example: batterycap set 80%s\n", colorRed, colorReset)
			os.Exit(1)
		}
		targetLimit, err := strconv.Atoi(os.Args[2])
		if err != nil || targetLimit < 80 || targetLimit > 100 {
			fmt.Fprintf(os.Stderr, "%s[!] Invalid limit '%s'. Supported range: 80 - 100%%%s\n", colorRed, os.Args[2], colorReset)
			os.Exit(1)
		}
		if err := setMCLLimit(targetLimit); err != nil {
			fmt.Fprintf(os.Stderr, "%s[!] Failed to set limit: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		if targetLimit < 100 {
			saveSavedLimit(targetLimit)
		}
		fmt.Printf("%s[✓] Successfully set native charge limit to %d%%.%s\n", colorGreen, targetLimit, colorReset)
		fmt.Printf("%s    macOS powerd will cap charging at %d%% and engage hardware passthrough.%s\n", colorGray, targetLimit, colorReset)

	case "off", "disable":
		if err := disableMCL(); err != nil {
			fmt.Fprintf(os.Stderr, "%s[!] Failed to disable limiter: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		fmt.Printf("%s[✓] Native charge limit disabled. Normal full charging restored (100%%).%s\n", colorYellow, colorReset)

	case "charge-to-full", "override":
		mcl, errMcl := getMCLInfo()
		if errMcl == nil && mcl.Limit >= 80 && mcl.Limit < 100 {
			saveSavedLimit(mcl.Limit)
		}
		if err := overrideFull(); err != nil {
			fmt.Fprintf(os.Stderr, "%s[!] Failed to override limit: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		fmt.Printf("%s[✓] Session override active: charging to 100%% once without altering saved limit.%s\n", colorCyan, colorReset)

	case "cancel-calibration", "uncalibrate", "stop-calibration", "abort-calibration", "cancel-override", "unoverride", "resume":
		targetLimit := 80
		if saved := loadSavedLimit(); saved >= 80 && saved < 100 {
			targetLimit = saved
		} else {
			mcl, errMcl := getMCLInfo()
			if errMcl == nil && mcl.Limit >= 80 && mcl.Limit < 100 {
				targetLimit = mcl.Limit
			}
		}
		if len(os.Args) >= 3 {
			n, err := strconv.Atoi(os.Args[2])
			if err != nil || n < 80 || n > 99 {
				fmt.Fprintf(os.Stderr, "%s[!] Invalid limit '%s'. Supported range: 80 - 99%%%s\n", colorRed, os.Args[2], colorReset)
				os.Exit(1)
			}
			targetLimit = n
		}
		saveSavedLimit(targetLimit)
		res, err := cancelCalibration(targetLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s[!] Failed to cancel calibration / override: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		hw, _ := getHardwareInfo()
		if res == 2 {
			fmt.Printf("%s[✓] Gas Gauge Calibration Complete — 100%% Reference Settled%s\n", colorGreen, colorReset)
			fmt.Printf("    macOS PMU has captured the full charge impedance reference point.\n")
			fmt.Printf("    Hardware passthrough is active at 100%% (0 mA draw). Battery charging is halted.\n\n")
			fmt.Printf("    %sArmed Target:%s      %s%d%%%s (hardware passthrough will automatically cap at %d%%)\n", colorBold, colorReset, colorCyan, targetLimit, colorReset, targetLimit)
			if len(hw.Cells) > 0 {
				var cellStrs []string
				for i, c := range hw.Cells {
					cellStrs = append(cellStrs, fmt.Sprintf("C%d: %.2fV (%d mAh)", i+1, c.VoltageV, c.QmaxMah))
				}
				fmt.Printf("    %s3-Cell Telemetry:%s %s\n", colorBold, colorReset, strings.Join(cellStrs, " | "))
			}
		} else if res == 1 {
			fmt.Printf("%s[ℹ] Active Gas Gauge Recalibration in Progress%s\n", colorYellow, colorReset)
			fmt.Printf("    macOS powerd has initiated a periodic battery calibration charge (ChargingUpForGauging).\n")
			fmt.Printf("    Apple Silicon's PMU requires an occasional 100%% reference cycle (~every 72h) to recalibrate\n")
			fmt.Printf("    its impedance track and prevent gas gauge percentage drift.\n\n")
			fmt.Printf("    %sStatus:%s            Target limit is armed and saved at %s%d%%%s. macOS powerd locks manual overrides\n", colorBold, colorReset, colorCyan, targetLimit, colorReset)
			fmt.Printf("                      during calibration. Once the PMU finishes calibration at 100%%, charging will halt and\n")
			fmt.Printf("                      hardware passthrough at %d%% will automatically resume.\n", targetLimit)
			if hw.AvgTimeToFullMinutes > 0 && hw.AvgTimeToFullMinutes < 60000 {
				fmt.Printf("    %sEstimated Time:%s   ~%d min remaining until 100%% reference\n", colorBold, colorReset, hw.AvgTimeToFullMinutes)
			}
			if hw.FullChargeCapacity > hw.RemainingCapacity && hw.RemainingCapacity > 0 {
				fmt.Printf("    %sCapacity Needed:%s  %d mAh remaining to full (%d / %d mAh)\n",
					colorBold, colorReset, hw.FullChargeCapacity-hw.RemainingCapacity, hw.RemainingCapacity, hw.FullChargeCapacity)
			}
			if len(hw.Cells) > 0 {
				var cellStrs []string
				for i, c := range hw.Cells {
					cellStrs = append(cellStrs, fmt.Sprintf("C%d: %.2fV (%d mAh)", i+1, c.VoltageV, c.QmaxMah))
				}
				fmt.Printf("    %s3-Cell Telemetry:%s %s\n", colorBold, colorReset, strings.Join(cellStrs, " | "))
			}
		} else {
			fmt.Printf("%s[✓] Calibration / override cancelled. Limit locked to %d%% (hardware passthrough active).%s\n", colorGreen, targetLimit, colorReset)
			fmt.Printf("%s    macOS powerd/PowerUI session target overridden. Mac will not charge to 100%%.%s\n", colorGray, colorReset)
			if len(hw.Cells) > 0 {
				var cellStrs []string
				for i, c := range hw.Cells {
					cellStrs = append(cellStrs, fmt.Sprintf("C%d: %.2fV (%d mAh)", i+1, c.VoltageV, c.QmaxMah))
				}
				fmt.Printf("    %s3-Cell Telemetry:%s %s\n", colorBold, colorReset, strings.Join(cellStrs, " | "))
			}
		}

	case "limits":
		mcl, err := getMCLInfo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s[!] Failed to query limits: %v%s\n", colorRed, err, colorReset)
			os.Exit(1)
		}
		fmt.Printf("\n%sSupported Native Hardware Limits:%s\n", colorBold, colorReset)
		fmt.Printf("  Supported Range: 80%% – 100%% (Any 1%% integer supported)\n")
		fmt.Printf("  System Presets:  [ 80%%, 85%%, 90%%, 95%%, 100%% ]\n")
		if mcl.Enabled {
			fmt.Printf("  Active Limit:    %d%%\n", mcl.Limit)
		} else {
			fmt.Printf("  Active Limit:    100%% (Limiter disabled)\n")
		}
		fmt.Println()

	case "watch":
		runWatchMode()

	case "help", "-h", "--help":
		printHelp()

	default:
		// Check if argument is a direct number (e.g. `batterycap 80`)
		if n, err := strconv.Atoi(cmd); err == nil {
			if err := setMCLLimit(n); err != nil {
				fmt.Fprintf(os.Stderr, "%s[!] %v%s\n", colorRed, err, colorReset)
				os.Exit(1)
			}
			if n >= 80 && n < 100 {
				saveSavedLimit(n)
			}
			fmt.Printf("%s[✓] Native charge limit set to %d%% (MCL active).%s\n", colorGreen, n, colorReset)
			fmt.Printf("%s    macOS powerd will cap charging at %d%% and engage hardware passthrough.%s\n", colorGray, n, colorReset)
			return
		}

		fmt.Fprintf(os.Stderr, "%sUnknown command '%s'. Run 'batterycap help' for command list.%s\n", colorRed, cmd, colorReset)
		os.Exit(1)
	}
}

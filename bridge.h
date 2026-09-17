#ifndef BRIDGE_H
#define BRIDGE_H

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    char deviceName[64];
    char healthCondition[32];
    int cycleCount;
    int currentCapacity;
    int healthMaxPercent;     // True battery health maximum capacity (e.g. 87%)
    int nominalCapacity;      // Nominal mAh
    int designCapacity;       // Design mAh
    int fullChargeCapacity;   // Full charge capacity mAh (from BatteryData)
    long long lastCalibrationTimestamp; // Last FullPathUpdated timestamp (epoch seconds)
    bool fullyCharged;        // Battery reports fully charged
    int voltageMv;
    int amperageMa;
    bool isCharging;
    bool externalConnected;
    bool isPassthrough;
    int uiState;              // 18 = ChargingUpForGauging (calibration in progress)
} BatteryHardwareInfo;

// Queries AppleSmartBattery from IOKit and system power profiler
int BatteryGetHardwareInfo(BatteryHardwareInfo *outInfo);

// Queries PowerUI private framework for MCL status & UI state
int BatteryGetMCLStatus(int *outEnabled, int *outLimit, int *outSupported, int *outLimits, int *outLimitsCount, int *outUIState);

// Sets MCL limit (80, 85, 90, 95, 100)
int BatterySetMCLLimit(unsigned char limit, char *errBuf, int errBufLen);

// Disables MCL (charging allowed to 100%)
int BatteryDisableMCL(char *errBuf, int errBufLen);

// Temporarily overrides MCL to 100% for current session
int BatteryOverrideFull(char *errBuf, int errBufLen);

// Cancels an active 100% session override or checks gas gauge calibration state.
// Returns: 0 on successful override cancellation, 1 if system calibration is locked by powerd, -1 on error.
int BatteryCancelCalibration(unsigned char limit, char *errBuf, int errBufLen);

#ifdef __cplusplus
}
#endif

#endif // BRIDGE_H

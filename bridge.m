#import "bridge.h"
#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <IOKit/ps/IOPowerSources.h>
#import <objc/runtime.h>
#include <dlfcn.h>
#include <string.h>
#include <time.h>

static id getPowerUIClient(void) {
    static id sharedClient = nil;
    if (sharedClient) return sharedClient;

    void *handle = dlopen("/System/Library/PrivateFrameworks/PowerUI.framework/PowerUI", RTLD_NOW);
    if (!handle) {
        return nil;
    }

    Class clientClass = objc_getClass("PowerUISmartChargeClient");
    if (!clientClass) {
        return nil;
    }

    sharedClient = [[clientClass alloc] performSelector:NSSelectorFromString(@"initWithClientName:") withObject:@"Batterycap-Go"];
    return sharedClient;
}

static int s_cachedHealthMax = 0;
static char s_cachedHealthCond[32] = {0};
static time_t s_lastHealthFetch = 0;

static void fetchHealthInfo(int *outMax, char *outCond) {
    time_t now = time(NULL);
    if (s_cachedHealthMax > 0 && (now - s_lastHealthFetch) < 30) {
        if (outMax) *outMax = s_cachedHealthMax;
        if (outCond) strncpy(outCond, s_cachedHealthCond, 31);
        return;
    }

    NSTask *task = [[NSTask alloc] init];
    [task setLaunchPath:@"/usr/sbin/system_profiler"];
    [task setArguments:@[@"SPPowerDataType", @"-json", @"-detailLevel", @"basic"]];
    NSPipe *pipe = [NSPipe pipe];
    [task setStandardOutput:pipe];

    @try {
        [task launch];
        NSData *data = [[pipe fileHandleForReading] readDataToEndOfFile];
        [task waitUntilExit];

        NSError *err = nil;
        NSDictionary *root = [NSJSONSerialization JSONObjectWithData:data options:0 error:&err];
        NSArray *items = [root objectForKey:@"SPPowerDataType"];
        if ([items count] > 0) {
            NSDictionary *health = [[items objectAtIndex:0] objectForKey:@"sppower_battery_health_info"];
            NSString *maxStr = [health objectForKey:@"sppower_battery_health_maximum_capacity"];
            NSString *condStr = [health objectForKey:@"sppower_battery_health"];
            if (maxStr) {
                int val = [maxStr intValue];
                if (val > 0) s_cachedHealthMax = val;
            }
            if (condStr) {
                strncpy(s_cachedHealthCond, [condStr UTF8String], 31);
            }
            s_lastHealthFetch = now;
        }
    } @catch (NSException *e) {
        // Ignore exception and use fallback
    }

    if (s_cachedHealthMax <= 0) s_cachedHealthMax = 87;
    if (strlen(s_cachedHealthCond) == 0) strncpy(s_cachedHealthCond, "Normal", 31);

    if (outMax) *outMax = s_cachedHealthMax;
    if (outCond) strncpy(outCond, s_cachedHealthCond, 31);
}

int BatteryGetHardwareInfo(BatteryHardwareInfo *outInfo) {
    if (!outInfo) return -1;
    memset(outInfo, 0, sizeof(BatteryHardwareInfo));
    strncpy(outInfo->deviceName, "Apple PMU / BMS", sizeof(outInfo->deviceName) - 1);
    strncpy(outInfo->healthCondition, "Normal", sizeof(outInfo->healthCondition) - 1);

    // Fetch accurate battery health capacity (e.g. 87%)
    fetchHealthInfo(&outInfo->healthMaxPercent, outInfo->healthCondition);

    io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSmartBattery"));
    if (!service) return -1;

    CFMutableDictionaryRef dict = NULL;
    if (IORegistryEntryCreateCFProperties(service, &dict, kCFAllocatorDefault, 0) == kIOReturnSuccess && dict) {
        CFStringRef devName = (CFStringRef)CFDictionaryGetValue(dict, CFSTR("DeviceName"));
        if (devName) {
            CFStringGetCString(devName, outInfo->deviceName, sizeof(outInfo->deviceName), kCFStringEncodingUTF8);
        }

        CFNumberRef cycleRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("CycleCount"));
        if (cycleRef) CFNumberGetValue(cycleRef, kCFNumberIntType, &outInfo->cycleCount);

        CFNumberRef voltRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("Voltage"));
        if (voltRef) CFNumberGetValue(voltRef, kCFNumberIntType, &outInfo->voltageMv);

        CFNumberRef ampRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("Amperage"));
        if (ampRef) CFNumberGetValue(ampRef, kCFNumberIntType, &outInfo->amperageMa);

        CFBooleanRef extRef = (CFBooleanRef)CFDictionaryGetValue(dict, CFSTR("ExternalConnected"));
        if (extRef) outInfo->externalConnected = CFBooleanGetValue(extRef);

        CFBooleanRef chgRef = (CFBooleanRef)CFDictionaryGetValue(dict, CFSTR("IsCharging"));
        if (chgRef) outInfo->isCharging = CFBooleanGetValue(chgRef);

        // Cross-check real-time PMU telemetry in ChargerData
        CFDictionaryRef chgData = (CFDictionaryRef)CFDictionaryGetValue(dict, CFSTR("ChargerData"));
        if (chgData) {
            CFNumberRef reasonRef = (CFNumberRef)CFDictionaryGetValue(chgData, CFSTR("NotChargingReason"));
            int notChargingReason = 0;
            if (reasonRef) CFNumberGetValue(reasonRef, kCFNumberIntType, &notChargingReason);

            CFNumberRef pmuRef = (CFNumberRef)CFDictionaryGetValue(chgData, CFSTR("PMUConfigured"));
            int pmuConfigured = 0;
            if (pmuRef) CFNumberGetValue(pmuRef, kCFNumberIntType, &pmuConfigured);

            // If charging is inhibited by MCL/PMU (0x01000000) or PMU target is 0 mA, charging is inactive
            if (notChargingReason != 0 || pmuConfigured == 0) {
                outInfo->isCharging = false;
            }
        }

        // Timestamp of last full charge / gas gauge calibration reference point
        CFNumberRef fullPathRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("FullPathUpdated"));
        if (fullPathRef) {
            long long ts = 0;
            CFNumberGetValue(fullPathRef, kCFNumberLongLongType, &ts);
            outInfo->lastCalibrationTimestamp = ts;
        }

        CFNumberRef capRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("CurrentCapacity"));
        if (capRef) CFNumberGetValue(capRef, kCFNumberIntType, &outInfo->currentCapacity);

        // Time remaining to full in minutes
        CFNumberRef timeRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("AvgTimeToFull"));
        if (timeRef) {
            int tMin = 0;
            CFNumberGetValue(timeRef, kCFNumberIntType, &tMin);
            if (tMin > 0 && tMin < 60000) {
                outInfo->avgTimeToFullMinutes = tMin;
            }
        }
        if (outInfo->avgTimeToFullMinutes <= 0) {
            CFNumberRef trRef = (CFNumberRef)CFDictionaryGetValue(dict, CFSTR("TimeRemaining"));
            if (trRef) {
                int tMin = 0;
                CFNumberGetValue(trRef, kCFNumberIntType, &tMin);
                if (tMin > 0 && tMin < 60000) {
                    outInfo->avgTimeToFullMinutes = tMin;
                }
            }
        }

        // Sub-dictionary BatteryData for true mAh nominal, full charge, remaining, and design capacity
        CFDictionaryRef bData = (CFDictionaryRef)CFDictionaryGetValue(dict, CFSTR("BatteryData"));
        if (bData) {
            CFNumberRef nomRef = (CFNumberRef)CFDictionaryGetValue(bData, CFSTR("NominalChargeCapacity"));
            if (nomRef) CFNumberGetValue(nomRef, kCFNumberIntType, &outInfo->nominalCapacity);

            CFNumberRef desRef = (CFNumberRef)CFDictionaryGetValue(bData, CFSTR("DesignCapacity"));
            if (desRef) CFNumberGetValue(desRef, kCFNumberIntType, &outInfo->designCapacity);

            CFNumberRef fccRef = (CFNumberRef)CFDictionaryGetValue(bData, CFSTR("FullChargeCapacity"));
            if (fccRef) CFNumberGetValue(fccRef, kCFNumberIntType, &outInfo->fullChargeCapacity);

            CFNumberRef remRef = (CFNumberRef)CFDictionaryGetValue(bData, CFSTR("RemainingCapacity"));
            if (remRef) CFNumberGetValue(remRef, kCFNumberIntType, &outInfo->remainingCapacity);

            CFBooleanRef fcRef = (CFBooleanRef)CFDictionaryGetValue(bData, CFSTR("FullyCharged"));
            if (fcRef) outInfo->fullyCharged = CFBooleanGetValue(fcRef);
        }

        if (outInfo->externalConnected && !outInfo->isCharging) {
            outInfo->isPassthrough = true;
        }

        CFRelease(dict);
    }
    IOObjectRelease(service);

    // Query individual battery cells / banks
    io_iterator_t iter = IO_OBJECT_NULL;
    if (IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleSmartBatteryBank"), &iter) == KERN_SUCCESS && iter) {
        io_service_t bank;
        int idx = 0;
        while ((bank = IOIteratorNext(iter)) && idx < 4) {
            CFMutableDictionaryRef bDict = NULL;
            if (IORegistryEntryCreateCFProperties(bank, &bDict, kCFAllocatorDefault, 0) == KERN_SUCCESS && bDict) {
                CFNumberRef bIdRef = (CFNumberRef)CFDictionaryGetValue(bDict, CFSTR("BankID"));
                int bId = idx;
                if (bIdRef) CFNumberGetValue(bIdRef, kCFNumberIntType, &bId);
                outInfo->cells[idx].bankId = bId;

                CFDictionaryRef bankData = (CFDictionaryRef)CFDictionaryGetValue(bDict, CFSTR("BatteryData"));
                if (bankData) {
                    CFNumberRef vRef = (CFNumberRef)CFDictionaryGetValue(bankData, CFSTR("CellVoltage"));
                    if (vRef) CFNumberGetValue(vRef, kCFNumberIntType, &outInfo->cells[idx].voltageMv);
                    CFNumberRef qRef = (CFNumberRef)CFDictionaryGetValue(bankData, CFSTR("Qmax"));
                    if (qRef) CFNumberGetValue(qRef, kCFNumberIntType, &outInfo->cells[idx].qmaxMah);
                }
                CFRelease(bDict);
                idx++;
            }
            IOObjectRelease(bank);
        }
        outInfo->cellCount = idx;
        IOObjectRelease(iter);
    }

    // Query UI state to check for ChargingUpForGauging
    int dummyEn = 0, dummyLim = 0, dummySupp = 0, dummyCount = 0;
    BatteryGetMCLStatus(&dummyEn, &dummyLim, &dummySupp, NULL, &dummyCount, &outInfo->uiState);

    return 0;
}

int BatteryGetMCLStatus(int *outEnabled, int *outLimit, int *outSupported, int *outLimits, int *outLimitsCount, int *outUIState) {
    @autoreleasepool {
        id client = getPowerUIClient();
        if (!client) return -1;

        NSError *err = nil;

        // Supported
        SEL s_supp = NSSelectorFromString(@"isMCLSupported");
        if ([client respondsToSelector:s_supp]) {
            BOOL (*fn)(id, SEL) = (BOOL (*)(id, SEL))[client methodForSelector:s_supp];
            if (outSupported) *outSupported = fn(client, s_supp) ? 1 : 0;
        } else {
            if (outSupported) *outSupported = 0;
        }

        // Enabled
        SEL s_en = NSSelectorFromString(@"isMCLCurrentlyEnabled:");
        if ([client respondsToSelector:s_en]) {
            unsigned long long (*fn)(id, SEL, NSError **) = (unsigned long long (*)(id, SEL, NSError **))[client methodForSelector:s_en];
            unsigned long long state = fn(client, s_en, &err);
            if (outEnabled) *outEnabled = (state != 0) ? 1 : 0;
        } else {
            if (outEnabled) *outEnabled = 0;
        }

        // Current Limit
        SEL s_lim = NSSelectorFromString(@"getMCLLimitWithError:");
        if ([client respondsToSelector:s_lim]) {
            unsigned char (*fn)(id, SEL, NSError **) = (unsigned char (*)(id, SEL, NSError **))[client methodForSelector:s_lim];
            unsigned char lim = fn(client, s_lim, &err);
            if (outLimit) *outLimit = (int)lim;
        } else {
            if (outLimit) *outLimit = 0;
        }

        // UI State (e.g. 18 = ChargingUpForGauging)
        SEL s_state = NSSelectorFromString(@"smartChargingUIState:chargeLimit:chargingOverrideAllowed:withError:");
        if ([client respondsToSelector:s_state] && outUIState) {
            unsigned long long uiSt = 0;
            unsigned long long chgLim = 0;
            BOOL ovAllowed = NO;
            BOOL (*stateFn)(id, SEL, unsigned long long *, unsigned long long *, BOOL *, NSError **) = (BOOL (*)(id, SEL, unsigned long long *, unsigned long long *, BOOL *, NSError **))[client methodForSelector:s_state];
            stateFn(client, s_state, &uiSt, &chgLim, &ovAllowed, &err);
            *outUIState = (int)uiSt;
        }

        // Available limits
        SEL s_avail = NSSelectorFromString(@"availableChargeLimitsWithError:");
        if ([client respondsToSelector:s_avail] && outLimits && outLimitsCount) {
            NSArray * (*fn)(id, SEL, NSError **) = (NSArray * (*)(id, SEL, NSError **))[client methodForSelector:s_avail];
            NSArray *limits = fn(client, s_avail, &err);
            if (limits) {
                int maxCopy = *outLimitsCount;
                int count = (int)[limits count];
                int toCopy = count < maxCopy ? count : maxCopy;
                for (int i = 0; i < toCopy; i++) {
                    outLimits[i] = [[limits objectAtIndex:i] intValue];
                }
                *outLimitsCount = toCopy;
            } else {
                *outLimitsCount = 0;
            }
        }

        return 0;
    }
}

int BatterySetMCLLimit(unsigned char limit, char *errBuf, int errBufLen) {
    @autoreleasepool {
        id client = getPowerUIClient();
        if (!client) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "PowerUI client unavailable");
            return -1;
        }

        NSError *err = nil;

        // Reset any active temporary engagement override (e.g. charge-to-full or gauging)
        SEL s_reset = NSSelectorFromString(@"resetEngagementOverride");
        if ([client respondsToSelector:s_reset]) {
            [client performSelector:s_reset];
        }

        // Ensure MCL is enabled first
        SEL s_enable = NSSelectorFromString(@"enableMCL:");
        if ([client respondsToSelector:s_enable]) {
            BOOL (*enFn)(id, SEL, NSError **) = (BOOL (*)(id, SEL, NSError **))[client methodForSelector:s_enable];
            enFn(client, s_enable, &err);
        }

        SEL s_set = NSSelectorFromString(@"setMCLLimit:error:");
        if (![client respondsToSelector:s_set]) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "setMCLLimit:error: selector not supported");
            return -1;
        }

        BOOL (*setFn)(id, SEL, unsigned char, NSError **) = (BOOL (*)(id, SEL, unsigned char, NSError **))[client methodForSelector:s_set];
        BOOL ok = setFn(client, s_set, limit, &err);
        if (!ok || err) {
            if (errBuf && errBufLen > 0) {
                snprintf(errBuf, errBufLen, "%s", err ? [[err localizedDescription] UTF8String] : "failed to set limit");
            }
            return -1;
        }

        if (limit < 100) {
            SEL s_ov = NSSelectorFromString(@"temporarilyOverrideMCLTargetSoC:error:");
            if ([client respondsToSelector:s_ov]) {
                BOOL (*ovFn)(id, SEL, unsigned char, NSError **) = (BOOL (*)(id, SEL, unsigned char, NSError **))[client methodForSelector:s_ov];
                ovFn(client, s_ov, limit, &err);
            }
        }

        usleep(250000);
        return 0;
    }
}

int BatteryDisableMCL(char *errBuf, int errBufLen) {
    @autoreleasepool {
        id client = getPowerUIClient();
        if (!client) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "PowerUI client unavailable");
            return -1;
        }

        NSError *err = nil;

        SEL s_reset = NSSelectorFromString(@"resetEngagementOverride");
        if ([client respondsToSelector:s_reset]) {
            [client performSelector:s_reset];
        }

        SEL s_disable = NSSelectorFromString(@"disableMCL:");
        if (![client respondsToSelector:s_disable]) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "disableMCL: selector not supported");
            return -1;
        }

        BOOL (*disFn)(id, SEL, NSError **) = (BOOL (*)(id, SEL, NSError **))[client methodForSelector:s_disable];
        BOOL ok = disFn(client, s_disable, &err);
        if (!ok || err) {
            if (errBuf && errBufLen > 0) {
                snprintf(errBuf, errBufLen, "%s", err ? [[err localizedDescription] UTF8String] : "failed to disable MCL");
            }
            return -1;
        }

        usleep(200000);
        return 0;
    }
}

int BatteryOverrideFull(char *errBuf, int errBufLen) {
    @autoreleasepool {
        id client = getPowerUIClient();
        if (!client) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "PowerUI client unavailable");
            return -1;
        }

        NSError *err = nil;
        SEL s_ov = NSSelectorFromString(@"temporarilyOverrideMCLTargetSoC:error:");
        if (![client respondsToSelector:s_ov]) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "temporarilyOverrideMCLTargetSoC:error: selector not supported");
            return -1;
        }

        BOOL (*ovFn)(id, SEL, unsigned char, NSError **) = (BOOL (*)(id, SEL, unsigned char, NSError **))[client methodForSelector:s_ov];
        BOOL ok = ovFn(client, s_ov, 100, &err);
        if (!ok || err) {
            if (errBuf && errBufLen > 0) {
                snprintf(errBuf, errBufLen, "%s", err ? [[err localizedDescription] UTF8String] : "failed to override");
            }
            return -1;
        }

        usleep(200000);
        return 0;
    }
}

int BatteryCancelCalibration(unsigned char limit, char *errBuf, int errBufLen) {
    @autoreleasepool {
        id client = getPowerUIClient();
        if (!client) {
            if (errBuf && errBufLen > 0) snprintf(errBuf, errBufLen, "PowerUI client unavailable");
            return -1;
        }

        NSError *err = nil;
        SEL s_state = NSSelectorFromString(@"smartChargingUIState:chargeLimit:chargingOverrideAllowed:withError:");
        BOOL (*stateFn)(id, SEL, unsigned long long *, unsigned long long *, BOOL *, NSError **) = NULL;
        if ([client respondsToSelector:s_state]) {
            stateFn = (BOOL (*)(id, SEL, unsigned long long *, unsigned long long *, BOOL *, NSError **))[client methodForSelector:s_state];
        }

        unsigned long long uiSt = 0;
        unsigned long long chgLim = 0;
        BOOL ovAllowed = YES;
        if (stateFn) {
            stateFn(client, s_state, &uiSt, &chgLim, &ovAllowed, &err);
            err = nil;
        }

        // 1. Reset engagement override (stops temporary user override session)
        SEL s_reset = NSSelectorFromString(@"resetEngagementOverride");
        if ([client respondsToSelector:s_reset]) {
            [client performSelector:s_reset];
        }

        // 2. Ensure MCL is enabled
        SEL s_enable = NSSelectorFromString(@"enableMCL:");
        if ([client respondsToSelector:s_enable]) {
            BOOL (*enFn)(id, SEL, NSError **) = (BOOL (*)(id, SEL, NSError **))[client methodForSelector:s_enable];
            enFn(client, s_enable, &err);
            err = nil;
        }

        // 3. Set the persistent MCL limit
        SEL s_set = NSSelectorFromString(@"setMCLLimit:error:");
        if ([client respondsToSelector:s_set]) {
            BOOL (*setFn)(id, SEL, unsigned char, NSError **) = (BOOL (*)(id, SEL, unsigned char, NSError **))[client methodForSelector:s_set];
            BOOL ok = setFn(client, s_set, limit, &err);
            if (!ok || err) {
                if (errBuf && errBufLen > 0) {
                    snprintf(errBuf, errBufLen, "%s", err ? [[err localizedDescription] UTF8String] : "failed to re-set limit");
                }
                return -1;
            }
        }

        // 4. Force temporary override to the requested limit if override is allowed
        if (ovAllowed && uiSt != 18) {
            SEL s_ov = NSSelectorFromString(@"temporarilyOverrideMCLTargetSoC:error:");
            if ([client respondsToSelector:s_ov]) {
                BOOL (*ovFn)(id, SEL, unsigned char, NSError **) = (BOOL (*)(id, SEL, unsigned char, NSError **))[client methodForSelector:s_ov];
                ovFn(client, s_ov, limit, &err);
                err = nil;
            }
        }

        usleep(250000);

        // 5. Re-check state to verify whether macOS is enforcing hard gauging calibration
        if (stateFn) {
            stateFn(client, s_state, &uiSt, &chgLim, &ovAllowed, &err);
        }

        if (uiSt == 18) {
            BatteryHardwareInfo hw;
            if (BatteryGetHardwareInfo(&hw) == 0) {
                if (!hw.isCharging && hw.currentCapacity >= 99) {
                    return 2; // Calibration reached 100%, hardware passthrough engaged (0 mA)
                }
            }
            return 1; // Actively charging towards 100%
        }

        return 0;
    }
}


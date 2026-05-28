package service

import (
	"strconv"
	"strings"
)

// Fork addition: local-currency + USD exchange-rate + display-discount
// settings (used by the discount-preview feature). Helpers and hook
// functions live here so setting_service.go's diff vs upstream stays
// minimal even as origin/main churns the surrounding parser/builder code.

const (
	defaultUSDExchangeRate = 7.2
	defaultLocalCurrency   = "CNY"
)

// SupportedLocalCurrencies is the whitelist for the LocalCurrency setting.
var SupportedLocalCurrencies = []string{"CNY", "HKD"}

// IsSupportedLocalCurrency checks if a currency code is in the whitelist
// (case-insensitive, leading/trailing whitespace tolerated).
func IsSupportedLocalCurrency(code string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	for _, c := range SupportedLocalCurrencies {
		if c == normalized {
			return true
		}
	}
	return false
}

// NormalizeLocalCurrency canonicalises arbitrary user input into a whitelist
// value, falling back to defaultLocalCurrency when the input is invalid.
func NormalizeLocalCurrency(code string) string {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if IsSupportedLocalCurrency(normalized) {
		return normalized
	}
	return defaultLocalCurrency
}

// parseUSDExchangeRate parses the stored string value, falling back to
// defaultUSDExchangeRate on invalid/missing input.
func parseUSDExchangeRate(raw string) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && v > 0 {
		return v
	}
	return defaultUSDExchangeRate
}

// applyDiscountCurrencyPublic populates the discount/currency fields on a
// PublicSettings from the raw settings map. Called from GetPublicSettings.
func applyDiscountCurrencyPublic(result *PublicSettings, settings map[string]string) {
	result.DisplayDiscountEnabled = settings[SettingKeyDisplayDiscountEnabled] == "true"
	result.LocalCurrency = NormalizeLocalCurrency(settings[SettingKeyLocalCurrency])
	result.USDExchangeRate = parseUSDExchangeRate(settings[SettingKeyUSDExchangeRate])
}

// publicDiscountCurrencyKeys are the SettingKey* names that must be added to
// the GetPublicSettings batched lookup. Returned as a slice so the caller can
// `append` without learning the names.
func publicDiscountCurrencyKeys() []string {
	return []string{
		SettingKeyDisplayDiscountEnabled,
		SettingKeyLocalCurrency,
		SettingKeyUSDExchangeRate,
	}
}

// applyDiscountCurrencyParse populates discount/currency fields on a
// SystemSettings from the raw map. Called from parseSettings.
func applyDiscountCurrencyParse(result *SystemSettings, settings map[string]string) {
	result.DisplayDiscountEnabled = settings[SettingKeyDisplayDiscountEnabled] == "true"
	result.LocalCurrency = NormalizeLocalCurrency(settings[SettingKeyLocalCurrency])
	result.USDExchangeRate = parseUSDExchangeRate(settings[SettingKeyUSDExchangeRate])
}

// applyDiscountCurrencyUpdates writes the discount/currency fields into the
// updates map. Called from buildSystemSettingsUpdates.
func applyDiscountCurrencyUpdates(updates map[string]string, settings *SystemSettings) {
	updates[SettingKeyDisplayDiscountEnabled] = strconv.FormatBool(settings.DisplayDiscountEnabled)
	updates[SettingKeyLocalCurrency] = NormalizeLocalCurrency(settings.LocalCurrency)
	if settings.USDExchangeRate > 0 {
		updates[SettingKeyUSDExchangeRate] = strconv.FormatFloat(settings.USDExchangeRate, 'f', 4, 64)
	}
}

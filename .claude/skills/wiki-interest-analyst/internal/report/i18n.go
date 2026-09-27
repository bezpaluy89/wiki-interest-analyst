package report

import "strings"

// Minimal two-language (uk/en) string tables. Report prose is generated from
// these plus computed metrics, so it never depends on the model inventing text.

var strings2 = map[string]map[string]string{
	"uk": {
		"doc_title":   "Аналіз інтересу за переглядами Wikipedia",
		"period":      "Період",
		"granularity": "Деталізація",
		"findings":    "Висновки",
		"limitations": "Припущення та обмеження",
		"trust":       "довіра",
		"leader":      "Найшвидше зростання",
		"leader_rel":  "Найкраща відносна динаміка",
		"baseline":    "база",
		"bucket":      "міс",
		"growing":     "зростання",
		"declining":   "спад",
		"flat":        "стабільно",
		"source":      "Джерело: Wikimedia Pageviews API (agent=user). Інтерес ≠ готовність платити.",
	},
	"en": {
		"doc_title":   "Wikipedia Pageview Interest Analysis",
		"period":      "Period",
		"granularity": "Granularity",
		"findings":    "Findings",
		"limitations": "Assumptions & limitations",
		"trust":       "trust",
		"leader":      "Fastest growth",
		"leader_rel":  "Best relative trend",
		"baseline":    "baseline",
		"bucket":      "bucket",
		"growing":     "growth",
		"declining":   "decline",
		"flat":        "flat",
		"source":      "Source: Wikimedia Pageviews API (agent=user). Interest ≠ willingness to pay.",
	},
}

var trustLabels = map[string]map[string]string{
	"uk": {"high": "висока", "medium": "середня", "low": "низька"},
	"en": {"high": "high", "medium": "medium", "low": "low"},
}

// reasonCodes maps trust reason codes to human text per language.
var reasonCodes = map[string]map[string]string{
	"uk": {
		"low_baseline":        "мала база переглядів — сигнал шумний",
		"strong_baseline":     "велика база переглядів",
		"gaps":                "є пропуски в даних",
		"weak_fit":            "тренд погано лягає на пряму (низький R²)",
		"not_significant":     "зростання статистично незначуще",
		"spike_driven":        "зростання спричинене сплеском, а не стійким трендом",
		"strong_signal":       "чіткий і надійний сигнал",
		"no_data":             "немає даних",
		"leader_low_trust":    "лідер за зростанням має низьку довіру — потрібна перевірка",
		"leader_spike_driven": "у лідера зростання спричинене сплеском",
	},
	"en": {
		"low_baseline":        "low view baseline — noisy signal",
		"strong_baseline":     "high view baseline",
		"gaps":                "data has gaps",
		"weak_fit":            "trend fits a line poorly (low R²)",
		"not_significant":     "growth is not statistically significant",
		"spike_driven":        "growth is driven by a spike, not a sustained trend",
		"strong_signal":       "clear, reliable signal",
		"no_data":             "no data",
		"leader_low_trust":    "growth leader has low trust — verify before acting",
		"leader_spike_driven": "growth leader's rise is spike-driven",
	},
}

func tr(lang, key string) string {
	if m, ok := strings2[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}

	return strings2["en"][key]
}

func trustText(lang, label string) string {
	if m, ok := trustLabels[lang]; ok {
		if v, ok := m[label]; ok {
			return v
		}
	}

	return label
}

func reasonText(lang, code string) string {
	if m, ok := reasonCodes[lang]; ok {
		if v, ok := m[code]; ok {
			return v
		}
	}

	return ""
}

func reasonsText(lang string, codes []string) string {
	var parts []string
	for _, c := range codes {
		if t := reasonText(lang, c); t != "" {
			parts = append(parts, t)
		}
	}

	return strings.Join(parts, "; ")
}

func limitationLines(lang string) []string {
	if lang == "en" {
		return []string{
			"Pageview interest is a proxy for attention, not for willingness to pay.",
			"Data starts 2015-07 and lags ~1 day; recent buckets may be incomplete.",
			"Language sections differ in size, so absolute views are not directly comparable.",
			"The chosen article title affects results; confirm the resolved title fits the topic.",
			"agent=user filters bots but cannot remove all automated traffic.",
		}
	}

	return []string{
		"Інтерес за переглядами — проксі уваги, а не готовності платити.",
		"Дані з 2015-07 і мають затримку ~1 день; останні періоди можуть бути неповними.",
		"Мовні розділи різні за розміром — абсолютні перегляди не порівнюються напряму.",
		"Обрана назва статті впливає на результат; перевір, чи вона відповідає темі.",
		"agent=user відсіює ботів, але не всі автоматичні перегляди.",
	}
}

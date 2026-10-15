package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	DefaultLang = "zh-CN"
	bundle      = map[string]map[string]string{}
	supported   []string
)

// Init 加载 locales/*.json 语言资源文件。
func Init() {
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		panic("i18n: " + err.Error())
	}
	for _, e := range entries {
		lang := strings.TrimSuffix(e.Name(), ".json")
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			continue
		}
		var m map[string]string
		if json.Unmarshal(data, &m) == nil {
			bundle[lang] = m
			supported = append(supported, lang)
		}
	}
	sort.Strings(supported)
}

// Supported 返回已支持的语言列表。
func Supported() []string {
	return append([]string(nil), supported...)
}

// Detect 解析 Accept-Language，返回首个支持的语言；否则返回默认语言。
func Detect(acceptLanguage string) string {
	for _, part := range strings.Split(acceptLanguage, ",") {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if lang == "" {
			continue
		}
		base := strings.ToLower(strings.SplitN(lang, "-", 2)[0])
		for _, s := range supported {
			if strings.ToLower(strings.SplitN(s, "-", 2)[0]) == base {
				return s
			}
		}
	}
	return DefaultLang
}

// T 按语言翻译 key，缺省回退到默认语言，最终回退为 key 本身。
func T(lang, key string, args ...interface{}) string {
	if m, ok := bundle[lang]; ok {
		if v, ok := m[key]; ok {
			return format(v, args)
		}
	}
	if lang != DefaultLang {
		if m, ok := bundle[DefaultLang]; ok {
			if v, ok := m[key]; ok {
				return format(v, args)
			}
		}
	}
	return key
}

func format(v string, args []interface{}) string {
	if len(args) == 0 {
		return v
	}
	return fmt.Sprintf(v, args...)
}

// FormatTime 按语言本地化时间格式。
func FormatTime(lang string, t time.Time) string {
	base := strings.ToLower(strings.SplitN(lang, "-", 2)[0])
	if base == "en" {
		return t.Format("Jan 2, 2006 15:04:05")
	}
	return t.Format("2006-01-02 15:04:05")
}

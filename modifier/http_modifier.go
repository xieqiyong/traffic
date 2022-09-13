package modifier

import (
	"fmt"
	"perfma-replay/proto"
	"regexp"
)
// 此处可添加更改流量更改规则
type HTTPModifierConfig struct {
	URLNegativeRegexp      HTTPURLRegexp              `json:"http-disallow-url"`
	URLRegexp              HTTPURLRegexp              `json:"http-allow-url"`
}

type HTTPURLRegexp []urlRegexp

type HTTPModifier struct {
	config *HTTPModifierConfig
}

type urlRegexp struct {
	regexp *regexp.Regexp
}


func (r *HTTPURLRegexp) Set(s string) error {
	regexp, err := regexp.Compile(s)
	*r = append(*r, urlRegexp{regexp: regexp})
	return err
}

func (r *HTTPURLRegexp) String() string {
	return fmt.Sprint(*r)
}

func NewHTTPModifier(config *HTTPModifierConfig) *HTTPModifier {
	// Optimization to skip modifier completely if we do not need it
	if len(config.URLRegexp) == 0 &&
		len(config.URLNegativeRegexp) == 0 {
		return nil
	}

	return &HTTPModifier{config: config}
}

func (m *HTTPModifier) Rewrite(payload []byte) (response []byte) {
	if !proto.HasRequestTitle(payload) {
		return payload
	}
	if len(m.config.URLRegexp) > 0 {
		path := proto.Path(payload)

		matched := false

		for _, f := range m.config.URLRegexp {
			if f.regexp.Match(path) {
				matched = true
				break
			}
		}

		if !matched {
			return
		}
	}

	if len(m.config.URLNegativeRegexp) > 0 {
		path := proto.Path(payload)

		for _, f := range m.config.URLNegativeRegexp {
			if f.regexp.Match(path) {
				return
			}
		}
	}
	return payload
}

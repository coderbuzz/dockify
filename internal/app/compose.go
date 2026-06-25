package app

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Services map[string]yaml.Node `yaml:"services"`
}

func parseServiceNames(compose string) ([]string, error) {
	var cf composeFile
	if err := yaml.Unmarshal([]byte(compose), &cf); err != nil {
		return nil, fmt.Errorf("parse compose: %w", err)
	}
	if len(cf.Services) == 0 {
		return nil, fmt.Errorf("compose has no services defined")
	}
	names := make([]string, 0, len(cf.Services))
	for name := range cf.Services {
		names = append(names, name)
	}
	return names, nil
}

func getServiceName(compose string) string {
	names, err := parseServiceNames(compose)
	if err != nil || len(names) == 0 {
		return "app"
	}
	return names[0]
}

func generateCompose(image string, port int, envVars string, volumes string) string {
	compose := fmt.Sprintf(`services:
  app:
    image: %s
    restart: unless-stopped
    networks:
      - dockify`, image)

	if envVars != "" {
		compose += "\n    environment:"
		for _, kv := range splitEnvVars(envVars) {
			compose += fmt.Sprintf("\n      - %s", kv)
		}
	}

	if port > 0 {
		compose += fmt.Sprintf("\n    expose:\n      - \"%d\"", port)
	}

	if volumes != "" {
		compose += "\n    volumes:"
		for _, vol := range splitEnvVars(volumes) {
			compose += fmt.Sprintf("\n      - %s", vol)
		}
	}

	compose += `

networks:
  dockify:
    external: true`

	return compose
}

func ensureDockifyNetwork(compose string) string {
	if strings.Contains(compose, "dockify") {
		return compose
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return compose
	}

	services, ok := doc["services"].(map[string]interface{})
	if !ok {
		return compose
	}

	for name := range services {
		svc, _ := services[name].(map[string]interface{})
		if svc == nil {
			svc = make(map[string]interface{})
			services[name] = svc
		}
		nets, _ := svc["networks"].([]interface{})
		svc["networks"] = append(nets, "dockify")
	}

	doc["networks"] = map[string]interface{}{
		"dockify": map[string]interface{}{
			"external": true,
		},
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return compose
	}
	return string(out)
}

func splitEnvVars(envVars string) []string {
	var result []string
	lines := strings.Split(envVars, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, ",") {
			for _, kv := range strings.Split(line, ",") {
				kv = strings.TrimSpace(kv)
				if kv != "" {
					result = append(result, kv)
				}
			}
		} else {
			result = append(result, line)
		}
	}
	return result
}

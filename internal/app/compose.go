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

func appNetworkAlias(name string) string {
	r := strings.NewReplacer(".", "-", "_", "-", " ", "-")
	return r.Replace(name)
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

func ensureDockifyNetwork(compose string, appName string) string {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return compose
	}

	services, ok := doc["services"].(map[string]interface{})
	if !ok {
		return compose
	}

	alias := appNetworkAlias(appName)

	for name := range services {
		svc, _ := services[name].(map[string]interface{})
		if svc == nil {
			svc = make(map[string]interface{})
			services[name] = svc
		}

		nets := getNetworksList(svc)

		found := false
		for i, net := range nets {
			switch n := net.(type) {
			case string:
				if n == "dockify" {
					nets[i] = map[string]interface{}{
						"dockify": mergeDockifyCfg(nil, alias),
					}
					found = true
				}
			case map[string]interface{}:
				if cfg, ok := n["dockify"]; ok {
					if cfgMap, ok := cfg.(map[string]interface{}); ok {
						n["dockify"] = mergeDockifyCfg(cfgMap, alias)
					} else {
						n["dockify"] = mergeDockifyCfg(nil, alias)
					}
					found = true
				}
			}
		}

		if !found {
			nets = append(nets, map[string]interface{}{
				"dockify": mergeDockifyCfg(nil, alias),
			})
		}

		svc["networks"] = nets
	}

	if _, ok := doc["networks"].(map[string]interface{}); !ok {
		doc["networks"] = map[string]interface{}{
			"dockify": map[string]interface{}{
				"external": true,
			},
		}
	} else {
		networks := doc["networks"].(map[string]interface{})
		if _, ok := networks["dockify"]; !ok {
			networks["dockify"] = map[string]interface{}{
				"external": true,
			}
		}
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return compose
	}
	return string(out)
}

func getNetworksList(svc map[string]interface{}) []interface{} {
	netsRaw, ok := svc["networks"]
	if !ok {
		return nil
	}

	switch nets := netsRaw.(type) {
	case []interface{}:
		return nets
	case map[string]interface{}:
		result := make([]interface{}, 0, len(nets))
		for netName, netCfg := range nets {
			if netCfg == nil {
				result = append(result, netName)
			} else {
				result = append(result, map[string]interface{}{netName: netCfg})
			}
		}
		return result
	default:
		return nil
	}
}

func mergeDockifyCfg(existing map[string]interface{}, alias string) map[string]interface{} {
	if existing == nil {
		existing = make(map[string]interface{})
	}
	aliases, _ := existing["aliases"].([]interface{})
	existing["aliases"] = append(aliases, alias)
	return existing
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

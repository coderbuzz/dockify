package app

import (
	"fmt"
	"strconv"
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

func generateCompose(image string, port int, volumes string, appName string, memoryLimit, cpuLimit, logMaxSize, logMaxFile string, envKeys []string, command string, ports string) string {
	svcName := "app"
	if appName != "" {
		svcName = sanitizeAppName(appName)
	}
	compose := fmt.Sprintf(`services:
  %s:
    image: %s`, svcName, image)

	compose += "\n    restart: unless-stopped"

	if command != "" {
		compose += "\n    command: " + command
	}

	if ports != "" {
		compose += "\n    ports:"
		for _, p := range splitEnvVars(ports) {
			compose += fmt.Sprintf("\n      - %s", p)
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

	compose += "\n    networks:\n      - dockify"

	if memoryLimit != "" {
		compose += fmt.Sprintf("\n    mem_limit: %s", memoryLimit)
	}

	if cpuLimit != "" {
		compose += fmt.Sprintf("\n    cpus: %s", cpuLimit)
	}

	if logMaxSize != "" || logMaxFile != "" {
		compose += "\n    logging:"
		compose += "\n      driver: json-file"
		compose += "\n      options:"
		if logMaxSize != "" {
			compose += fmt.Sprintf("\n        max-size: \"%s\"", logMaxSize)
		}
		if logMaxFile != "" {
			compose += fmt.Sprintf("\n        max-file: \"%s\"", logMaxFile)
		}
	}

	if len(envKeys) > 0 {
		compose += "\n    environment:"
		for _, k := range envKeys {
			if strings.TrimSpace(k) != "" {
				compose += fmt.Sprintf("\n      - %s=${%s}", k, k)
			}
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

func sanitizeAppName(name string) string {
	r := strings.NewReplacer(".", "-", "_", "-", " ", "-")
	return r.Replace(name)
}

func (a *App) ContainerServiceName() string {
	if a.ComposeMode == "simple" {
		return sanitizeAppName(a.Name)
	}
	return getServiceName(a.Compose)
}

func renameFirstService(compose string, newName string) string {
	if !strings.Contains(compose, "services:") {
		return compose
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return compose
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return compose
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return compose
	}

	var servicesNode *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "services" {
			servicesNode = root.Content[i+1]
			break
		}
	}

	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode || len(servicesNode.Content) < 2 {
		return compose
	}

	firstKey := servicesNode.Content[0]
	if firstKey.Value == newName {
		return compose
	}

	firstKey.Value = newName

	out, err := yaml.Marshal(&doc)
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

type simpleFields struct {
	Image      string
	Port       int
	EnvKeys    []string
	Volumes    string
	MemoryLimit string
	CPULimit    string
	LogMaxSize  string
	LogMaxFile  string
	Command     string
	Ports       string
}

func parseSimpleFields(compose string) simpleFields {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return simpleFields{}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return simpleFields{}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return simpleFields{}
	}

	// Find services node
	var servicesNode *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "services" {
			servicesNode = root.Content[i+1]
			break
		}
	}
	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode || len(servicesNode.Content) < 2 {
		return simpleFields{}
	}

	// Take the first service
	svc := servicesNode.Content[1]
	if svc.Kind != yaml.MappingNode {
		return simpleFields{}
	}

	var sf simpleFields
	for i := 0; i+1 < len(svc.Content); i += 2 {
		key := svc.Content[i].Value
		val := svc.Content[i+1]

		switch key {
		case "image":
			sf.Image = val.Value
		case "expose":
			if val.Kind == yaml.SequenceNode && len(val.Content) > 0 {
				portStr := strings.Trim(val.Content[0].Value, "\"")
				if p, err := strconv.Atoi(portStr); err == nil {
					sf.Port = p
				}
			}
		case "ports":
			if val.Kind == yaml.SequenceNode {
				var lines []string
				for _, item := range val.Content {
					if item.Value != "" {
						lines = append(lines, item.Value)
					}
				}
				sf.Ports = strings.Join(lines, "\n")
			}
		case "environment":
			if val.Kind == yaml.SequenceNode {
				var keys []string
				for _, item := range val.Content {
					v := item.Value
					if v == "" {
						continue
					}
					// Match ${KEY} or KEY=value patterns
					if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
						keys = append(keys, strings.TrimSuffix(strings.TrimPrefix(v, "${"), "}"))
					} else if idx := strings.Index(v, "="); idx > 0 {
						keys = append(keys, v[:idx])
					}
				}
				sf.EnvKeys = keys
			}
		case "volumes":
			if val.Kind == yaml.SequenceNode {
				var lines []string
				for _, item := range val.Content {
					if item.Value != "" {
						lines = append(lines, item.Value)
					}
				}
				sf.Volumes = strings.Join(lines, "\n")
			}
		case "mem_limit":
			sf.MemoryLimit = val.Value
		case "cpus":
			sf.CPULimit = val.Value
		case "command":
			sf.Command = val.Value
		case "logging":
			if val.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(val.Content); j += 2 {
				switch val.Content[j].Value {
				case "options":
					if val.Content[j+1].Kind != yaml.MappingNode {
						continue
					}
					for k := 0; k+1 < len(val.Content[j+1].Content); k += 2 {
						switch val.Content[j+1].Content[k].Value {
						case "max-size":
							sf.LogMaxSize = strings.Trim(val.Content[j+1].Content[k+1].Value, "\"")
						case "max-file":
							sf.LogMaxFile = strings.Trim(val.Content[j+1].Content[k+1].Value, "\"")
						}
					}
				}
			}
		}
	}

	return sf
}

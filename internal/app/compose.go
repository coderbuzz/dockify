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

func generateCompose(image string, port int, envVars string, volumes string, appName string) string {
	svcName := "app"
	if appName != "" {
		svcName = sanitizeAppName(appName)
	}
	compose := fmt.Sprintf(`services:
  %s:
    image: %s
    restart: unless-stopped
    networks:
      - dockify`, svcName, image)

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

func sanitizeAppName(name string) string {
	r := strings.NewReplacer(".", "-", "_", "-", " ", "-")
	return r.Replace(name)
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
	Image   string
	Port    int
	EnvVars string
	Volumes string
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
		case "environment":
			if val.Kind == yaml.SequenceNode {
				var lines []string
				for _, item := range val.Content {
					if item.Value != "" {
						lines = append(lines, item.Value)
					}
				}
				sf.EnvVars = strings.Join(lines, "\n")
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
		}
	}

	return sf
}

package app

import (
	"cmp"
	"fmt"
	"slices"
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

type simpleCompose struct {
	Services map[string]simpleService   `yaml:"services"`
	Networks map[string]map[string]bool `yaml:"networks"`
}

type simpleService struct {
	Image       string         `yaml:"image,omitempty"`
	Restart     string         `yaml:"restart,omitempty"`
	Command     string         `yaml:"command,omitempty"`
	Ports       []string       `yaml:"ports,omitempty"`
	Expose      []string       `yaml:"expose,omitempty"`
	Volumes     []string       `yaml:"volumes,omitempty"`
	Networks    []string       `yaml:"networks,omitempty"`
	MemLimit    *yaml.Node     `yaml:"mem_limit,omitempty"`
	CPUs        *yaml.Node     `yaml:"cpus,omitempty"`
	Ulimits     *simpleUlimits `yaml:"ulimits,omitempty"`
	Logging     *simpleLogging `yaml:"logging,omitempty"`
	Environment []string       `yaml:"environment,omitempty"`
}

type simpleUlimits struct {
	Nofile struct {
		Soft *yaml.Node `yaml:"soft"`
		Hard *yaml.Node `yaml:"hard"`
	} `yaml:"nofile"`
}

type simpleLogging struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options,omitempty"`
}

// plain is an untagged scalar: written unquoted when that is valid YAML, so
// "cpus: 0.5" stays a number as before, and quoted otherwise.
func plain(s string) *yaml.Node {
	if s == "" {
		return nil
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Value: s}
}

func generateCompose(image string, port int, volumes string, appName string, memoryLimit, cpuLimit, logMaxSize, logMaxFile string, envKeys []string, command string, ports string, ulimitsNofile string) string {
	svcName := "app"
	if appName != "" {
		svcName = sanitizeAppName(appName)
	}
	svc := simpleService{
		Image:    image,
		Restart:  "unless-stopped",
		Command:  command,
		Ports:    splitEnvVars(ports),
		Volumes:  splitEnvVars(volumes),
		Networks: []string{"dockify"},
		MemLimit: plain(memoryLimit),
		CPUs:     plain(cpuLimit),
	}
	if port > 0 {
		svc.Expose = []string{strconv.Itoa(port)}
	}
	if ulimitsNofile != "" {
		svc.Ulimits = &simpleUlimits{}
		svc.Ulimits.Nofile.Soft = plain(ulimitsNofile)
		svc.Ulimits.Nofile.Hard = plain(ulimitsNofile)
	}
	if logMaxSize != "" || logMaxFile != "" {
		svc.Logging = &simpleLogging{Driver: "json-file", Options: map[string]string{}}
		if logMaxSize != "" {
			svc.Logging.Options["max-size"] = logMaxSize
		}
		if logMaxFile != "" {
			svc.Logging.Options["max-file"] = logMaxFile
		}
	}
	for _, k := range envKeys {
		if k = strings.TrimSpace(k); k != "" {
			svc.Environment = append(svc.Environment, k+"=${"+k+"}")
		}
	}

	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(simpleCompose{
		Services: map[string]simpleService{svcName: svc},
		Networks: map[string]map[string]bool{"dockify": {"external": true}},
	}); err != nil {
		return "" // unreachable: simpleCompose holds only encodable types
	}
	return b.String()
}

// ensureDockifyNetwork attaches the services to the external "dockify" network
// Caddy proxies through and declares that network. When some service already
// lists dockify the services are left alone: the others are kept off the
// shared network on purpose (a "db" alias there would collide across apps).
// It edits the YAML node tree, so comments and key order survive, and returns
// the input unchanged when nothing is missing.
func ensureDockifyNetwork(compose string) string {
	var doc yaml.Node
	if yaml.Unmarshal([]byte(compose), &doc) != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return compose
	}
	root := doc.Content[0]
	services := mapValue(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return compose
	}

	var missing []*yaml.Node
	for i := 1; i < len(services.Content); i += 2 {
		svc := services.Content[i]
		if svc.Kind != yaml.MappingNode || mapValue(svc, "network_mode") != nil {
			continue // network_mode cannot be combined with networks
		}
		if onDockify(svc) {
			missing = nil
			break
		}
		missing = append(missing, svc)
	}
	for _, svc := range missing {
		switch nets := mapValue(svc, "networks"); {
		case nets == nil:
			svc.Content = append(svc.Content, scalar("networks"), &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{scalar("dockify")}})
		case nets.Kind == yaml.MappingNode:
			nets.Content = append(nets.Content, scalar("dockify"), &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle})
		default:
			nets.Content = append(nets.Content, scalar("dockify"))
		}
	}
	changed := len(missing) > 0

	nets := mapValue(root, "networks")
	if nets == nil {
		nets = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, scalar("networks"), nets)
	} else if nets.Kind != yaml.MappingNode { // "networks:" left empty
		*nets = yaml.Node{Kind: yaml.MappingNode}
	}
	dockify := mapValue(nets, "dockify")
	if dockify == nil {
		dockify = &yaml.Node{Kind: yaml.MappingNode}
		nets.Content = append(nets.Content, scalar("dockify"), dockify)
	} else if dockify.Kind != yaml.MappingNode {
		*dockify = yaml.Node{Kind: yaml.MappingNode}
	}
	if ext := mapValue(dockify, "external"); ext == nil {
		dockify.Content = append(dockify.Content, scalar("external"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		changed = true
	} else if ext.Value != "true" {
		*ext = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		changed = true
	}

	if !changed {
		return compose
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return compose
	}
	return string(out)
}

// onDockify reports whether a service lists the dockify network, in either
// the sequence or the mapping form.
func onDockify(svc *yaml.Node) bool {
	nets := mapValue(svc, "networks")
	switch {
	case nets == nil:
		return false
	case nets.Kind == yaml.MappingNode:
		return mapValue(nets, "dockify") != nil
	default:
		return slices.ContainsFunc(nets.Content, func(n *yaml.Node) bool { return n.Value == "dockify" })
	}
}

// mapValue returns the value node of key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
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
	Image         string
	Port          int
	EnvKeys       []string
	Volumes       string
	MemoryLimit   string
	CPULimit      string
	LogMaxSize    string
	LogMaxFile    string
	Command       string
	Ports         string
	UlimitsNofile string
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
		case "ulimits":
			if val.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(val.Content); j += 2 {
					if val.Content[j].Value == "nofile" && val.Content[j+1].Kind == yaml.MappingNode {
						for k := 0; k+1 < len(val.Content[j+1].Content); k += 2 {
							if val.Content[j+1].Content[k].Value == "hard" {
								sf.UlimitsNofile = val.Content[j+1].Content[k+1].Value
							}
						}
					}
				}
			}
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

// dockerVolumes lists the named and anonymous Docker volumes used by the
// compose services; unlike bind mounts they live outside the app folder.
func dockerVolumes(compose string) []string {
	var cf struct {
		Services map[string]struct {
			Volumes []yaml.Node `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &cf); err != nil {
		return nil
	}
	var vols []string
	for _, svc := range cf.Services {
		for _, v := range svc.Volumes {
			switch v.Kind {
			case yaml.ScalarNode:
				src, _, hasTarget := strings.Cut(v.Value, ":")
				if !hasTarget {
					vols = append(vols, v.Value) // anonymous
				} else if strings.IndexAny(src, "./~$") != 0 {
					vols = append(vols, src)
				}
			case yaml.MappingNode:
				var m struct{ Type, Source, Target string }
				if v.Decode(&m) == nil && m.Type == "volume" {
					vols = append(vols, cmp.Or(m.Source, m.Target))
				}
			}
		}
	}
	slices.Sort(vols)
	return slices.Compact(vols)
}

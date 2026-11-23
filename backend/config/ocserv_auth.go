package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func (c *Config) LoadOcservAuth(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var method string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line, _, _ := strings.Cut(scanner.Text(), "#")
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"")
		switch strings.TrimSpace(key) {
		case "enable-auth":
			return fmt.Errorf("%s: remove enable-auth and configure a single authentication method", path)
		case "auth":
			if method != "" {
				return fmt.Errorf("%s: configure a single auth directive", path)
			}
			switch {
			case value == "certificate":
				method = "certificate"
			case strings.HasPrefix(value, "radius[") && strings.HasSuffix(value, "]"):
				method = "radius"
			default:
				return fmt.Errorf("%s: auth must be certificate or radius", path)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if method == "" {
		return fmt.Errorf("%s: missing auth directive", path)
	}
	c.RadiusAuth = method == "radius"
	return nil
}

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

	var method, alternative string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value, _, _ = strings.Cut(value, "#")
		value = strings.Trim(strings.TrimSpace(value), "\"")
		switch strings.TrimSpace(key) {
		case "enable-auth":
			if alternative != "" || value != "certificate" {
				return fmt.Errorf("%s: only enable-auth = certificate is supported alongside RADIUS", path)
			}
			alternative = value
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
	if alternative != "" && method != "radius" {
		return fmt.Errorf("%s: certificate as an alternative requires primary RADIUS auth", path)
	}
	c.RadiusAuth = method == "radius"
	c.CertificateAuth = method == "certificate" || alternative == "certificate"
	c.CustomerMultiNodeEnabled = c.RadiusAuth || getBoolEnv("CUSTOMER_MULTI_NODE_ENABLED", false)
	return nil
}

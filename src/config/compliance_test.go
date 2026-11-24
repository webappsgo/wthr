package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComplianceConfigDecodesStandardsAndDefaultsOff(t *testing.T) {
	var cfg AppConfig
	if err := yaml.Unmarshal([]byte("server:\n  compliance:\n    gdpr: true\n    pci_dss: true\n"), &cfg); err != nil {
		t.Fatalf("decode compliance config: %v", err)
	}
	if !cfg.Server.Compliance.GDPR || !cfg.Server.Compliance.PCIDSS || !cfg.Server.Compliance.Enabled() {
		t.Fatalf("decoded compliance config = %+v, want GDPR and PCI-DSS enabled", cfg.Server.Compliance)
	}
	if cfg.Server.Compliance.CCPA || cfg.Server.Compliance.HIPAA || cfg.Server.Compliance.SOC2 || cfg.Server.Compliance.ISO27001 ||
		cfg.Server.Compliance.FedRAMP || cfg.Server.Compliance.LGPD || cfg.Server.Compliance.PIPEDA || cfg.Server.Compliance.APPI || cfg.Server.Compliance.PDPA {
		t.Fatalf("unspecified compliance standards defaulted on: %+v", cfg.Server.Compliance)
	}
}

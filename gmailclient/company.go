package gmailclient

import (
	"os"
	"regexp"
	"strings"
)

func DetectCompany(atts []DownloadedAttachment, from string, subject string) string {
	// 1) XML primero (más confiable)
	for _, a := range atts {
		if strings.EqualFold(a.Ext, "xml") {
			if c := detectCompanyFromXML(a.LocalPath); c != "" {
				return c
			}
		}
	}

	// 2) dominio del From
	if c := companyFromEmail(from); c != "" {
		return c
	}

	// 3) Subject fallback
	if c := sanitizeCompany(subject); c != "desconocido" {
		return c
	}

	return "desconocido"
}

func detectCompanyFromXML(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(b)

	patterns := []string{
		`RazonSocial="([^"]+)"`,
		`Raz[oó]nSocial="([^"]+)"`,
		`Nombre="([^"]+)"`,
		`Name="([^"]+)"`,
		`Emisor[^>]*Nombre="([^"]+)"`,
		`Issuer[^>]*Name="([^"]+)"`,
		`Supplier[^>]*Name="([^"]+)"`,
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p)
		if m := re.FindStringSubmatch(s); len(m) == 2 {
			return sanitizeCompany(m[1])
		}
	}
	return ""
}

func companyFromEmail(from string) string {
	// "Amazon <billing@amazon.com>" -> amazon
	re := regexp.MustCompile(`@([a-zA-Z0-9.\-]+\.[a-zA-Z]{2,})`)
	m := re.FindStringSubmatch(from)
	if len(m) != 2 {
		return ""
	}
	domain := m[1]
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return sanitizeCompany(parts[len(parts)-2])
	}
	return sanitizeCompany(domain)
}

func sanitizeCompany(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "desconocido"
	}
	return s
}

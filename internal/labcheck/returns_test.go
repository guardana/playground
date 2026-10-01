package labcheck_test

import (
	"testing"

	"github.com/guardana/playground/runner/gateway"
)

// The classification is what docs/status.md and the bring-your-own-policy
// runbook say it is: every tool but the shell declares what it returns, the
// CRM, database and filesystem are trusted destinations and mail and the web
// untrusted ones, a page is untrusted and public, and a private read is
// trusted and confidential. A flow scenario's verdict turns on these values.
func TestTheClassificationDeclaresWhatTheDocsSay(t *testing.T) {
	var classes []gateway.Class
	readYAML(t, "config/gateway/classification.yaml", &classes)
	zones := map[string]string{
		"victim-crm": "TRUSTED_INTERNAL", "victim-db": "TRUSTED_INTERNAL", "victim-fs": "TRUSTED_INTERNAL",
		"victim-mail": "UNTRUSTED_EXTERNAL", "victim-web": "UNTRUSTED_EXTERNAL", "victim-shell": "",
	}
	reads := map[string]gateway.Returns{
		"victim-web/web.fetch":         {Trust: "UNTRUSTED_EXTERNAL", Sensitivity: "PUBLIC"},
		"victim-fs/fs.read":            {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-db/db.query":           {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-crm/crm.read_customer": {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
	}
	for _, class := range classes {
		name := class.Upstream + "/" + class.Tool
		if zone, known := zones[class.Upstream]; !known || class.TrustZone != zone {
			t.Errorf("%s has trust_zone %q, want %q", name, class.TrustZone, zone)
		}
		if declared := class.Returns != nil; declared != (class.Upstream != "victim-shell") {
			t.Errorf("%s declares returns: %t", name, declared)
		}
		if want, stated := reads[name]; stated && (class.Returns == nil || *class.Returns != want) {
			t.Errorf("%s returns %+v, want %+v", name, class.Returns, want)
		}
	}
	if len(classes) < len(reads) {
		t.Fatalf("read %d classes", len(classes))
	}
}

package labcheck_test

import (
	"testing"

	"github.com/guardana/playground/runner/gateway"
)

// Every tool but the shell declares what it returns, the CRM, database,
// filesystem and the payments a charge or a refund moves are trusted
// destinations, and mail, the web and a payout untrusted ones; a page is
// untrusted and public, and a private read is trusted and confidential. A flow
// scenario's verdict turns on these values, so changing one is a decision,
// never a side effect.
func TestTheClassificationDeclaresWhatFlowScenariosRelyOn(t *testing.T) {
	var classes []gateway.Class
	readYAML(t, "config/gateway/classification.yaml", &classes)
	zones := map[string]string{
		"victim-crm": "TRUSTED_INTERNAL", "victim-db": "TRUSTED_INTERNAL", "victim-fs": "TRUSTED_INTERNAL",
		"victim-mail": "UNTRUSTED_EXTERNAL", "victim-web": "UNTRUSTED_EXTERNAL", "victim-shell": "",
		"victim-pay": "TRUSTED_INTERNAL", "victim-pay/pay.payout": "UNTRUSTED_EXTERNAL",
	}
	reads := map[string]gateway.Returns{
		"victim-web/web.fetch":         {Trust: "UNTRUSTED_EXTERNAL", Sensitivity: "PUBLIC"},
		"victim-fs/fs.read":            {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-db/db.query":           {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-crm/crm.read_customer": {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-pay/pay.read_charge":   {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-pay/pay.charge":        {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-pay/pay.refund":        {Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"},
		"victim-pay/pay.payout":        {Trust: "TRUSTED_INTERNAL", Sensitivity: "INTERNAL"},
	}
	for _, class := range classes {
		name := class.Upstream + "/" + class.Tool
		zone, known := zones[name]
		if !known {
			zone, known = zones[class.Upstream]
		}
		if !known || class.TrustZone != zone {
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

// The effect class is what an obligation or an idempotency rule keys on: a
// money movement classified as a read would pass every rule written for
// TRANSACT. Every classified tool is named here, so a new one is a decision too.
func TestEveryClassifiedToolHasTheEffectItHas(t *testing.T) {
	var classes []gateway.Class
	readYAML(t, "config/gateway/classification.yaml", &classes)
	effects := map[string]string{
		"victim-crm/crm.read_customer": "READ", "victim-crm/crm.update_note": "WRITE",
		"victim-crm/crm.update_bank_account": "WRITE", "victim-crm/crm.export_table": "READ",
		"victim-db/db.query": "READ", "victim-db/db.execute": "WRITE", "victim-db/db.drop_table": "DELETE",
		"victim-fs/fs.read": "READ", "victim-fs/fs.write": "WRITE", "victim-fs/fs.list": "READ",
		"victim-shell/shell.exec": "EXECUTE",
		"victim-mail/mail.send":   "COMMUNICATE", "victim-mail/mail.send_bulk": "COMMUNICATE",
		"victim-pay/pay.charge": "TRANSACT", "victim-pay/pay.refund": "TRANSACT",
		"victim-pay/pay.payout": "TRANSACT", "victim-pay/pay.read_charge": "READ",
		"victim-web/web.fetch": "READ",
	}
	seen := make(map[string]bool, len(classes))
	for _, class := range classes {
		name := class.Upstream + "/" + class.Tool
		seen[name] = true
		if want, known := effects[name]; !known || class.Effect != want {
			t.Errorf("%s has effect %q, want %q", name, class.Effect, want)
		}
	}
	for name := range effects {
		if !seen[name] {
			t.Errorf("%s is not classified", name)
		}
	}
}

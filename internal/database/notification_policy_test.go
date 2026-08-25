package database

import "testing"

func TestNotificationDeliveryDisposition(t *testing.T) {
	for name, test := range map[string]struct {
		enabled     bool
		event       string
		maintenance bool
		status      string
		queued      bool
	}{
		"policy disabled":         {event: "dns.failed", status: "suppressed"},
		"failure enabled":         {enabled: true, event: "dns.failed", status: "pending", queued: true},
		"recovery enabled":        {enabled: true, event: "dns.recovered", status: "pending", queued: true},
		"maintenance suppression": {enabled: true, event: "dns.failed", maintenance: true, status: "suppressed"},
	} {
		t.Run(name, func(t *testing.T) {
			status, queued := notificationDeliveryDisposition(test.enabled, test.event, test.maintenance)
			if status != test.status || queued != test.queued {
				t.Fatalf("status=%q queued=%v", status, queued)
			}
		})
	}
}

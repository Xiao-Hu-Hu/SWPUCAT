package email

import "testing"

func TestSMTPAddress(t *testing.T) {
	for _, tc := range []struct {
		name, host, want string
		port             int
	}{
		{name: "hostname", host: "smtp.example.test", port: 587, want: "smtp.example.test:587"},
		{name: "ipv6", host: "2001:db8::1", port: 465, want: "[2001:db8::1]:465"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := smtpAddress(tc.host, tc.port); got != tc.want {
				t.Fatalf("smtpAddress(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
		})
	}
}

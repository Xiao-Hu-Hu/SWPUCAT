package email

import (
	"SWPUCAT/internal/infrastructure/config"
	"bytes"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"

	"github.com/yuin/goldmark"
)

type SMTPService struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPService(cfg *config.EmailConfig) *SMTPService {
	return &SMTPService{
		host:     cfg.SMTPHost,
		port:     cfg.SMTPPort,
		username: cfg.Username,
		password: cfg.Password,
		from:     cfg.From,
	}
}

func (s *SMTPService) SendVerificationCode(to string, code string) error {
	subject := "SWPUCAT - 邮箱验证码"
	body := fmt.Sprintf("您的验证码是：%s，有效期2分钟。如非本人操作，请忽略此邮件。", code)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		s.from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	log.Printf("[SMTP] Connecting to %s", addr)

	// Connect to SMTP server
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("[SMTP] Connection failed: %v", err)
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()
	log.Printf("[SMTP] Connected successfully")

	// Create SMTP client
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		log.Printf("[SMTP] Client creation failed: %v", err)
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Start TLS
	tlsConfig := &tls.Config{
		ServerName: s.host,
	}
	if err = client.StartTLS(tlsConfig); err != nil {
		log.Printf("[SMTP] TLS failed: %v", err)
		return fmt.Errorf("failed to start TLS: %w", err)
	}
	log.Printf("[SMTP] TLS started")

	// Authenticate
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err = client.Auth(auth); err != nil {
		log.Printf("[SMTP] Auth failed: %v", err)
		return fmt.Errorf("authentication failed: %w", err)
	}
	log.Printf("[SMTP] Authenticated")

	// Set sender and recipient
	if err = client.Mail(s.from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send data
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start data: %w", err)
	}
	_, err = writer.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close data: %w", err)
	}

	log.Printf("[SMTP] Email sent to %s", to)
	return client.Quit()
}

func markdownToHTML(md string) string {
	var buf bytes.Buffer
	if err := goldmark.New().Convert([]byte(md), &buf); err != nil {
		return "<p>" + md + "</p>"
	}
	return buf.String()
}

func (s *SMTPService) SendAnnouncementNotification(to string, title string, content string) error {
	subject := fmt.Sprintf("SWPUCAT - 公告通知：%s", title)
	htmlContent := markdownToHTML(content)
	body := fmt.Sprintf(`<div style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;max-width:640px;margin:0 auto;padding:20px;">
  <h2 style="color:#1a1a1a;border-bottom:2px solid #409eff;padding-bottom:10px;">%s</h2>
  <div style="line-height:1.7;color:#333;font-size:15px;">%s</div>
  <hr style="border:none;border-top:1px solid #eee;margin:24px 0;">
  <p style="color:#999;font-size:12px;">此邮件由 SWPUCAT 系统自动发送，请勿回复。</p>
</div>`, title, htmlContent)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		s.from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	log.Printf("[SMTP-Notify] Connecting to %s, recipient=%s", addr, to)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("[SMTP-Notify] Connection failed: %v", err)
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		log.Printf("[SMTP-Notify] Client creation failed: %v", err)
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	tlsConfig := &tls.Config{ServerName: s.host}
	if err = client.StartTLS(tlsConfig); err != nil {
		log.Printf("[SMTP-Notify] TLS failed: %v", err)
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err = client.Auth(auth); err != nil {
		log.Printf("[SMTP-Notify] Auth failed: %v", err)
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err = client.Mail(s.from); err != nil {
		log.Printf("[SMTP-Notify] Set sender failed: %v", err)
		return fmt.Errorf("failed to set sender: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		log.Printf("[SMTP-Notify] Set recipient failed: %v", err)
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		log.Printf("[SMTP-Notify] Data command failed: %v", err)
		return fmt.Errorf("failed to start data: %w", err)
	}
	_, err = writer.Write([]byte(msg))
	if err != nil {
		log.Printf("[SMTP-Notify] Write data failed: %v", err)
		return fmt.Errorf("failed to write data: %w", err)
	}
	err = writer.Close()
	if err != nil {
		log.Printf("[SMTP-Notify] Close data failed: %v", err)
		return fmt.Errorf("failed to close data: %w", err)
	}

	log.Printf("[SMTP-Notify] Message accepted by server, sending QUIT...")
	quitErr := client.Quit()
	if quitErr != nil {
		log.Printf("[SMTP-Notify] QUIT failed: %v", quitErr)
		return fmt.Errorf("failed to quit: %w", quitErr)
	}
	log.Printf("[SMTP-Notify] Successfully sent to %s", to)
	return nil
}

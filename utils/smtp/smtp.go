package smtp

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

type Client struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewClient(host string, port int, username, password, from string) *Client {
	return &Client{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

func (c *Client) SendVerificationCode(qqNumber int64, code string) error {
	to := fmt.Sprintf("%d@qq.com", qqNumber)
	subject := "Haruki Bot 验证码"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .code { font-size: 32px; font-weight: bold; color: #6366f1; letter-spacing: 4px; }
        .footer { margin-top: 20px; color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <h2>Haruki Bot 验证码</h2>
        <p>您的验证码是：</p>
        <p class="code">%s</p>
        <p>验证码有效期为 10 分钟，请勿泄露给他人。</p>
        <div class="footer">
            <p>如果您没有请求此验证码，请忽略此邮件。</p>
            <p>— Haruki Dev Team</p>
        </div>
    </div>
</body>
</html>
`, code)

	return c.sendMail(to, subject, body, true)
}

func (c *Client) sendMail(to, subject, body string, isHTML bool) error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	contentType := "text/plain"
	if isHTML {
		contentType = "text/html"
	}
	fromEmail := c.from
	if idx := strings.Index(c.from, "<"); idx != -1 {
		endIdx := strings.Index(c.from, ">")
		if endIdx > idx {
			fromEmail = c.from[idx+1 : endIdx]
		}
	}
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: %s; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", c.from, to, subject, contentType, body)
	auth := smtp.PlainAuth("", c.username, c.password, c.host)
	if c.port == 587 || c.port == 25 {
		return c.sendWithSTARTTLS(addr, auth, fromEmail, to, []byte(msg))
	}
	if c.port == 465 {
		return c.sendWithTLS(addr, auth, fromEmail, to, []byte(msg))
	}
	return c.sendWithSTARTTLS(addr, auth, fromEmail, to, []byte(msg))
}

func (c *Client) sendWithSTARTTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func (c *Client) sendWithTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName: c.host,
	}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial failed: %w", err)
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return fmt.Errorf("smtp client creation failed: %w", err)
	}
	defer client.Close()
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL command failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT command failed: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA command failed: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close failed: %w", err)
	}
	return client.Quit()
}

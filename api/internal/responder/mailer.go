package responder

import (
	"fmt"
	"observeddb-go-api/cfg"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type Mailer struct {
	mailInfo  cfg.MailerConfig
	metaInfo  cfg.MetaConfig
	debugMode bool
}

func NewMailer(mailerSettings cfg.MailerConfig, metaInfo cfg.MetaConfig, debug bool) *Mailer {
	return &Mailer{
		mailInfo:  mailerSettings,
		metaInfo:  metaInfo,
		debugMode: debug,
	}
}

func (m *Mailer) SendNewAccoundEmail(receiverName, receiver, token string, expirationTime time.Time) error {
	apiDomain := m.metaInfo.URL + m.metaInfo.Group
	body := newAccountMsg(receiverName,
		receiver,
		m.metaInfo.Name,
		apiDomain,
		token,
		expirationTime,
	)

	subject := fmt.Sprintf("New account created for %s", m.metaInfo.Name)
	err := m.send(subject, body, receiverName, receiver)
	if err != nil {
		return fmt.Errorf("cannot send email for new account: %w", err)
	}

	return nil
}

func newAccountMsg(fullName, email, apiName, apiDomain, token string, expirationTime time.Time) string {
	return fmt.Sprintf(
		`Dear %s,

An account for the %s was created.
To get started, please set your initial password by using this endpoint:

%s/user/password/init

with token: %s

As an example, you can use the following curl command:

curl -X POST %s/user/password/init \
-H "Content-Type: application/json" \
-d '{
	"token": "%s",
	"email": "%s",
	"password": "your_secure_password"
}'

Swagger documentation for the API can be found at %s/swagger

This token will expire at %s. If you need a new token, please contact the administrator.`,
		fullName, apiName, apiDomain, token, apiDomain, token, email, apiDomain, expirationTime.Format(time.RFC1123))
}

func (m *Mailer) send(subject, body, receiverName, receiver string) error {
	from := mail.NewEmail(m.metaInfo.Name, m.mailInfo.SendEmail)
	var toMail string
	if m.debugMode {
		toMail = m.mailInfo.SendEmail
	} else {
		toMail = receiver
	}
	to := mail.NewEmail(receiverName, toMail)

	message := mail.NewSingleEmailPlainText(from, subject, to, body)
	client := sendgrid.NewSendClient(m.mailInfo.APIKey)
	_, err := client.Send(message)
	if err != nil {
		return err //nolint:wrapcheck // no need to wrap since this an internal function
	}

	return nil
}

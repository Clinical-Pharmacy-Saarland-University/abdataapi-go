package responder

import (
	"fmt"
	"observeddb-go-api/cfg"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type Mailer struct {
	mailInfo      cfg.MailerConfig
	metaInfo      cfg.MetaConfig
	debugReceiver string
	debugMode     bool
}

func NewMailer(mailerSettings cfg.MailerConfig, metaInfo cfg.MetaConfig, debug bool) *Mailer {
	return &Mailer{
		mailInfo:      mailerSettings,
		metaInfo:      metaInfo,
		debugReceiver: mailerSettings.DebugReciver,
		debugMode:     debug,
	}
}

func (m *Mailer) SendNewAccoundEmail(fullName, receiver, token string) error {

	from := mail.NewEmail(m.metaInfo.Name, m.mailInfo.SendEmail)
	var toMail string
	if m.debugMode {
		toMail = m.debugReceiver
	} else {
		toMail = receiver
	}
	to := mail.NewEmail(fullName, toMail)

	subject := "Welcome to" + m.metaInfo.Name
	message := mail.NewSingleEmailPlainText(from, subject, to, "")
	client := sendgrid.NewSendClient(m.mailInfo.APIKey)
	_, err := client.Send(message)
	if err != nil {
		return fmt.Errorf("cannot send email: %w", err)
	}

	return nil

	// 	body := fmt.Sprintf(`Dear %s,

	// An account for the '%s' API service was created.
	// To get started, please set your initial password by using this endpoint:

	// https://yourdomain.com/user/password/init

	// with this token: %s

	// As an example, you can use the following curl command:

	// curl -X POST https://yourdomain.com/user/password/init \
	// -H "Content-Type: application/json" \
	// -d '{
	//     "token": "%s",
	//     "email": "%s",
	//     "password": "your_secure_password"
	// }'

	// This token will expire in %s. If you need a new token, please contact the administrator.`, role, token)

}

func newAccountMsg(fullName, token string) string {
	return fmt.Sprintf(`Dear %s,
	
	An account for the '%s' API service was created.
	To get started, please set your initial password by using this endpoint:

	https://yourdomain.com/user/password/init

	with this token: %s

	As an example, you can use the following curl command:

	curl -X POST https://yourdomain.com/user/password/init \
	-H "Content-Type: application/json" \
	-d '{
		"token": "%s",
		"email": "%s",
		"password": "your_secure_password"
	}'

	This token will expire in %s. If you need a new token, please contact the administrator.`)
}

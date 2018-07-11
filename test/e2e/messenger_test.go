package e2e_test

import (
	"os"

	"fmt"
	api "github.com/kubedb/user-manager/apis/users/v1alpha1"
	"github.com/kubedb/user-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"time"
)

var _ = Describe("Messenger", func() {
	var (
		f *framework.Invocation

		labels          map[string]string
		name, namespace string

		secret, notifierConfig *core.Secret

		messagingServiceObj, messagingService *api.MessagingService
		messageObj, message                   *api.Message

		drive                     string
		to                        []string
		msgBody, chat, email, sms string
		err                       error

		authTokenToSendMessage string
		authTokenToSeeHistory  string
		send, seeHist          bool
		skipMsg                string
		skipingInfo            []string

		timeBeforeSend string
	)

	BeforeEach(func() {
		f = root.Invoke()
		name = f.App()
		namespace = f.Namespace()
		labels = map[string]string{
			"app": f.App(),
		}

		secret = nil
		send = false
		seeHist = false
		msgBody = ""
		email = ""
		sms = ""
		chat = ""
		skipMsg = "Missing necessary tokens to"
		skipingInfo = []string{}
	})

	Describe("Send Message in Hipchat", func() {
		BeforeEach(func() {
			if authTokenToSendMessage, send = os.LookupEnv("AUTH_TOKEN_TO_SEND_MSG"); send {
				secret = f.NewSecret(name+"-notifier-config", namespace, authTokenToSendMessage, labels)
			}

			authTokenToSeeHistory, seeHist = os.LookupEnv("AUTH_TOKEN_TO_SEE_HIST")

			drive = "Hipchat"
			to = []string{"ops-alerts"}
			messagingServiceObj = f.NewMessagingService(name, namespace, labels, drive, secret.Name, to)

			chat = "test-msg: Hello world from appscode/messenger :D"
			messageObj = f.NewMessage(name+"-notify-hipchat", namespace, labels, name, msgBody, chat, email, sms)

			timeBeforeSend = framework.GetDateString(time.Now())
		})

		JustBeforeEach(func() {
			if send {
				By("Creating secret...")
				notifierConfig, err = f.CreateSecret(secret)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Creating CRDs...")
			messagingService, err = f.CreateMessagingService(messagingServiceObj)
			Expect(err).NotTo(HaveOccurred())

			message, err = f.CreateMessage(messageObj)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("To \"ops-alerts\" room", func() {
			AfterEach(func() {
				By("Deleting secrets...")
				err = f.DeleteAllSecrets()
				Expect(err).NotTo(HaveOccurred())

				By("Deleting CRDs...")
				err = f.DeleteAllCRDs()
				Expect(err).NotTo(HaveOccurred())
			})

			FIt("Should send message", func() {
				if !send || !seeHist {
					if !send {
						skipingInfo = append(skipingInfo, "send message")
					}
					if !seeHist {
						skipingInfo = append(skipingInfo, "see message")
					}
					Skip(fmt.Sprintf("%s %v", skipMsg, skipingInfo))
				}

				f.EventuallyCheckMessage(chat, timeBeforeSend, authTokenToSeeHistory).ShouldNot(HaveOccurred())
			})
		})
	})
})

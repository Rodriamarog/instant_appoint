export default function TermsOfService() {
  return (
    <div className="max-w-3xl mx-auto px-6 py-16">
      <h1 className="text-3xl font-bold mb-2">Terms of Service</h1>
      <p className="text-gray-500 mb-8">Last updated: May 16, 2026</p>

      <section className="space-y-6 text-gray-700">
        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">1. Acceptance of Terms</h2>
          <p>
            By accessing or using NeuroCrow ("the platform"), you agree to be bound by these Terms
            of Service. If you do not agree, do not use the platform.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">2. Description of Service</h2>
          <p>
            NeuroCrow is a SaaS platform that enables service businesses to manage appointment
            bookings and send automated reminders via WhatsApp using Meta's official WhatsApp
            Business Cloud API.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">3. Permitted Use</h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>You may only use NeuroCrow for lawful business purposes.</li>
            <li>You are responsible for all messages sent through your connected WhatsApp Business Account.</li>
            <li>You must comply with Meta's WhatsApp Business Policy and all applicable laws.</li>
            <li>You must obtain proper consent from your customers before sending them WhatsApp messages.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">4. Prohibited Use</h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>Sending spam, unsolicited messages, or bulk promotional content.</li>
            <li>Using the platform for any illegal activity.</li>
            <li>Violating Meta's WhatsApp Business Terms or Messaging Policy.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">5. Account Responsibility</h2>
          <p>
            You are responsible for maintaining the security of your account credentials and for
            all activity that occurs under your account.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">6. Opt-Out</h2>
          <p>
            Your customers may opt out of receiving WhatsApp messages at any time by replying STOP.
            You agree to honor all opt-out requests promptly.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">7. Disclaimer</h2>
          <p>
            NeuroCrow is provided "as is" without warranties of any kind. We are not liable for
            message delivery failures, WhatsApp API outages, or any indirect damages arising from
            use of the platform.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">8. Contact</h2>
          <p>
            For questions about these Terms, contact us at:{' '}
            <a href="mailto:support@neurocrow.com" className="text-blue-600 underline">
              support@neurocrow.com
            </a>
          </p>
        </div>
      </section>
    </div>
  )
}

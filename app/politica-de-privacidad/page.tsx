export default function PrivacyPolicy() {
  return (
    <div className="max-w-3xl mx-auto px-6 py-16">
      <h1 className="text-3xl font-bold mb-2">Privacy Policy</h1>
      <p className="text-gray-500 mb-8">Last updated: May 16, 2026</p>

      <section className="space-y-6 text-gray-700">
        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">1. About NeuroCrow</h2>
          <p>
            NeuroCrow ("we", "our", "us") is a SaaS platform that helps service businesses manage
            appointment bookings via WhatsApp. This Privacy Policy explains how we collect, use,
            and protect information when you use our platform.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">2. Information We Collect</h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>Account information: name, email address, and password when you register.</li>
            <li>WhatsApp Business Account details: phone number, WABA ID, and access tokens provided during setup.</li>
            <li>Message data: inbound and outbound WhatsApp messages processed through our platform for the purpose of appointment management.</li>
            <li>Calendar data: appointment details including client phone numbers, dates, and times.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">3. How We Use Your Information</h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>To operate the appointment booking and reminder features of the platform.</li>
            <li>To send WhatsApp messages on your behalf to your customers.</li>
            <li>To authenticate your WhatsApp Business Account via Meta's APIs.</li>
            <li>To improve the reliability and performance of our services.</li>
          </ul>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">4. WhatsApp Data</h2>
          <p>
            NeuroCrow integrates with the WhatsApp Business Platform via Meta's official Cloud API.
            Message content processed through our platform is used solely to facilitate appointment
            booking and reminders. We do not sell or share WhatsApp message data with third parties.
            Users may opt out of receiving WhatsApp messages by replying STOP.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">5. Data Retention</h2>
          <p>
            We retain message and appointment data for as long as your account is active. You may
            request deletion of your data at any time by contacting us.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">6. Third-Party Services</h2>
          <p>
            We use Meta's WhatsApp Business Platform to deliver messages. Your use of WhatsApp is
            also subject to Meta's Privacy Policy. We use PocketBase for data storage hosted on
            our own infrastructure.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">7. Contact</h2>
          <p>
            If you have questions about this Privacy Policy, contact us at:{' '}
            <a href="mailto:support@neurocrow.com" className="text-blue-600 underline">
              support@neurocrow.com
            </a>
          </p>
        </div>
      </section>
    </div>
  )
}

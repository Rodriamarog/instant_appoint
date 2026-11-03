/**
 * The public terms of service, served at /terminos-de-servicio.
 *
 * Registered with Meta's app review alongside the privacy policy; see
 * privacy-policy.tsx.
 */
export function TermsOfServicePage() {
  return (
    <div className="max-w-3xl mx-auto px-6 py-16">
      <h1 className="text-3xl font-bold mb-2">Términos de servicio</h1>
      <p className="text-gray-500 mb-8">
        Última actualización: 16 de mayo de 2026
      </p>

      <section className="space-y-6 text-gray-700">
        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            1. Aceptación de los términos
          </h2>
          <p>
            Al acceder a NeuroCrow («la plataforma») o usarla, aceptas quedar
            sujeto a estos términos de servicio. Si no estás de acuerdo con
            ellos, no uses la plataforma.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            2. Descripción del servicio
          </h2>
          <p>
            NeuroCrow es una plataforma SaaS que permite a los negocios de
            servicios gestionar sus citas y enviar recordatorios automáticos
            por WhatsApp, usando la Cloud API oficial de WhatsApp Business de
            Meta.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            3. Uso permitido
          </h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>Solo puedes usar NeuroCrow con fines comerciales lícitos.</li>
            <li>
              Eres responsable de todos los mensajes que salgan de la cuenta de
              WhatsApp Business que conectes.
            </li>
            <li>
              Debes cumplir la política de WhatsApp Business de Meta y toda la
              legislación aplicable.
            </li>
            <li>
              Debes obtener el consentimiento de tus clientes antes de
              enviarles mensajes por WhatsApp.
            </li>
          </ul>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            4. Uso prohibido
          </h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>Enviar spam, mensajes no solicitados o publicidad masiva.</li>
            <li>Usar la plataforma para cualquier actividad ilegal.</li>
            <li>
              Infringir los términos de WhatsApp Business de Meta o su
              política de mensajería.
            </li>
          </ul>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            5. Responsabilidad sobre la cuenta
          </h2>
          <p>
            Eres responsable de mantener seguras las credenciales de tu cuenta
            y de toda la actividad que ocurra con ella.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            6. Baja de los mensajes
          </h2>
          <p>
            Tus clientes pueden dejar de recibir mensajes de WhatsApp en
            cualquier momento respondiendo STOP. Te comprometes a atender esas
            bajas sin demora.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            7. Exención de responsabilidad
          </h2>
          <p>
            NeuroCrow se ofrece «tal cual», sin garantías de ningún tipo. No
            respondemos por mensajes que no lleguen, por caídas de la API de
            WhatsApp ni por ningún daño indirecto derivado del uso de la
            plataforma.
          </p>
        </div>

        <div>
          <h2 className="text-xl font-semibold text-gray-900 mb-2">
            8. Contacto
          </h2>
          <p>
            Si tienes dudas sobre estos términos, escríbenos a:{' '}
            <a href="mailto:support@neurocrow.com" className="text-blue-600 underline">
              support@neurocrow.com
            </a>
          </p>
        </div>
      </section>
    </div>
  )
}

import { Link } from 'react-router'

import { Logo } from '@/components/logo'

/**
 * The public privacy policy, served at /politica-de-privacidad.
 *
 * Meta's app review requires a reachable privacy-policy URL for the
 * WhatsApp Business product, and this path is the one registered with it, so
 * the route has to keep answering with the page rather than the SPA's 404.
 */
export function PrivacyPolicyPage() {
  return (
    <div className="max-w-3xl mx-auto px-6 py-10 sm:py-14">
      <Link to="/" className="inline-block mb-12">
        <Logo />
      </Link>
      <h1 className="font-heading text-4xl font-bold tracking-[-0.035em] mb-2">Política de privacidad</h1>
      <p className="text-muted-foreground mb-8">
        Última actualización: 16 de mayo de 2026
      </p>

      <section className="space-y-6 text-muted-foreground">
        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            1. Sobre NeuroCrow
          </h2>
          <p>
            NeuroCrow («nosotros») es una plataforma SaaS que ayuda a los
            negocios de servicios a gestionar sus citas por WhatsApp. Esta
            política de privacidad explica qué información recogemos, cómo la
            usamos y cómo la protegemos cuando usas la plataforma.
          </p>
        </div>

        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            2. Información que recogemos
          </h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>
              Datos de la cuenta: nombre, correo electrónico y contraseña al
              registrarte.
            </li>
            <li>
              Datos de la cuenta de WhatsApp Business: número de teléfono, ID
              de la WABA y los tokens de acceso que se entregan durante la
              conexión.
            </li>
            <li>
              Mensajes: los mensajes de WhatsApp entrantes y salientes que
              pasan por la plataforma, con el fin de gestionar las citas.
            </li>
            <li>
              Datos del calendario: los detalles de cada cita, incluidos el
              teléfono del cliente, la fecha y la hora.
            </li>
          </ul>
        </div>

        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            3. Cómo usamos tu información
          </h2>
          <ul className="list-disc pl-5 space-y-1">
            <li>
              Para operar la agenda de citas y los recordatorios de la
              plataforma.
            </li>
            <li>
              Para enviar mensajes de WhatsApp a tus clientes en tu nombre.
            </li>
            <li>
              Para autenticar tu cuenta de WhatsApp Business con las API de
              Meta.
            </li>
            <li>
              Para mejorar la fiabilidad y el rendimiento de nuestros
              servicios.
            </li>
          </ul>
        </div>

        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            4. Datos de WhatsApp
          </h2>
          <p>
            NeuroCrow se integra con la plataforma de WhatsApp Business a
            través de la Cloud API oficial de Meta. El contenido de los
            mensajes que pasan por la plataforma se usa únicamente para
            agendar citas y enviar recordatorios. No vendemos ni compartimos
            los mensajes de WhatsApp con terceros. Quien no quiera seguir
            recibiéndolos puede responder STOP.
          </p>
        </div>

        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            5. Conservación de los datos
          </h2>
          <p>
            Conservamos los mensajes y las citas mientras tu cuenta siga
            activa. Puedes pedir que borremos tus datos en cualquier momento
            escribiéndonos.
          </p>
        </div>

        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            6. Servicios de terceros
          </h2>
          <p>
            Usamos la plataforma de WhatsApp Business de Meta para entregar
            los mensajes. Tu uso de WhatsApp queda sujeto además a la
            política de privacidad de Meta. Para almacenar los datos usamos
            PocketBase, alojado en nuestra propia infraestructura.
          </p>
        </div>

        <div>
          <h2 className="font-heading text-xl font-bold tracking-tight text-foreground mb-2">
            7. Contacto
          </h2>
          <p>
            Si tienes dudas sobre esta política de privacidad, escríbenos a:{' '}
            <a href="mailto:support@neurocrow.com" className="text-brand underline">
              support@neurocrow.com
            </a>
          </p>
        </div>
      </section>
    </div>
  )
}

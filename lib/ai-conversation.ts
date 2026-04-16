import { GoogleGenerativeAI } from '@google/generative-ai'

const genai = new GoogleGenerativeAI(process.env.GEMINI_API_KEY!)

const model = genai.getGenerativeModel({ model: 'gemini-2.5-flash' })

export interface ConversationMessage {
  role: 'user' | 'model'
  content: string
}

export async function generateReply(
  history: ConversationMessage[],
  incomingMessage: string
): Promise<string> {
  const now = new Date().toLocaleString('en-US', { timeZone: 'America/Tijuana' })

  const systemPrompt = `You are a friendly appointment booking assistant for a business.
Today is ${now}.

Your job is to help customers book appointments through natural conversation.
Keep responses concise and conversational — this is WhatsApp, not email.
When the customer and business agree on a specific date and time, confirm the appointment clearly.
Reply in the same language the customer uses.`

  const chat = model.startChat({
    systemInstruction: { role: 'user', parts: [{ text: systemPrompt }] },
    history: history.map(m => ({
      role: m.role,
      parts: [{ text: m.content }],
    })),
  })

  const result = await chat.sendMessage(incomingMessage)
  return result.response.text()
}

import { decodeOk } from '../decode'
import { list } from '../knowledge/decode'
import type { Request } from '../request'
import type { StreamRequest } from '../stream'
import { event, message, session } from './decode'
import type { ChatEvent, SessionInput } from './types'
// 读模型包含服务端身份与时间戳；写入只发送用例接受的字段。
const sessionInput = ({ title, knowledge_base_ids, system_prompt, temperature, max_tokens, history_turns, retrieval_mode, retrieval_limit, similarity_threshold }: Partial<SessionInput>) => ({ title, knowledge_base_ids, system_prompt, temperature, max_tokens, history_turns, retrieval_mode, retrieval_limit, similarity_threshold })
export function createChatApi(request: Request, stream: StreamRequest) {
  const path = (id: string) => `/chat/sessions/${encodeURIComponent(id)}`
  return {
    chatSessions: (signal?: AbortSignal) => request('/chat/sessions', list(session), 'GET', undefined, signal),
    chatSession: (id: string, signal?: AbortSignal) => request(path(id), session, 'GET', undefined, signal),
    createChatSession: (input: Partial<SessionInput> = {}, signal?: AbortSignal) => request('/chat/sessions', session, 'POST', sessionInput(input), signal),
    saveChatSession: (id: string, input: SessionInput, signal?: AbortSignal) => request(path(id), session, 'PUT', sessionInput(input), signal),
    deleteChatSession: (id: string, signal?: AbortSignal) => request(path(id), decodeOk, 'DELETE', undefined, signal),
    chatMessages: (id: string, signal?: AbortSignal) => request(`${path(id)}/messages`, list(message), 'GET', undefined, signal),
    sendChatMessage: (id: string, content: string, onEvent: (event: ChatEvent) => void, signal?: AbortSignal) => stream(`${path(id)}/messages`, { content }, (type, value) => onEvent(event(type, value)), signal),
    stopChat: (id: string, signal?: AbortSignal) => request(`${path(id)}/stop`, decodeOk, 'POST', undefined, signal),
  }
}

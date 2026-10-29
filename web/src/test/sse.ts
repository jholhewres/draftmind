// Server-Sent Events for tests: a whole stream as one body, or a stream the
// test feeds one event at a time.

export const sseEvent = (name: string, data: unknown = {}) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`

export const sseBody = (events: [string, unknown?][]) => events.map(([name, data]) => sseEvent(name, data)).join('')

export const sseResponse = (events: [string, unknown?][]) =>
  new Response(sseBody(events), { headers: { 'Content-Type': 'text/event-stream' } })

export type FedStream = { response: Response; push: (name: string, data?: unknown) => void; end: () => void }

// A response whose events arrive when the test pushes them.
export function fedStream(): FedStream {
  let ctrl!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (ctrl = c) })
  const enc = new TextEncoder()
  return {
    response: new Response(body, { headers: { 'Content-Type': 'text/event-stream' } }),
    push: (name, data) => ctrl.enqueue(enc.encode(sseEvent(name, data))),
    end: () => ctrl.close(),
  }
}

export type Health = { status: string; database: string }

export async function fetchHealth(signal?: AbortSignal): Promise<Health> {
  const res = await fetch('/api/health', { signal })
  if (!res.ok && res.status !== 503) throw new Error(`health: HTTP ${res.status}`)
  return (await res.json()) as Health
}

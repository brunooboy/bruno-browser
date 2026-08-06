type BoundMethod = (...args: unknown[]) => Promise<unknown>

declare global {
  interface Window {
    go?: {
      main?: {
        Desktop?: Record<string, BoundMethod>
      }
    }
    runtime?: {
      EventsOn?: (name: string, callback: (...data: unknown[]) => void) => () => void
    }
  }
}

export function onRuntimeEvent<T>(name: string, callback: (data: T) => void) {
  return window.runtime?.EventsOn?.(name, (data) => callback(data as T)) ?? (() => undefined)
}

export const isDesktop = () => Boolean(window.go?.main?.Desktop)

export async function invoke<T>(method: string, ...args: unknown[]): Promise<T> {
  const target = window.go?.main?.Desktop?.[method]
  if (!target) {
    throw new Error('Esta operação exige o aplicativo desktop. A prévia web não possui acesso ao disco local.')
  }
  try {
    return await target(...args) as T
  } catch (error) {
    if (error instanceof Error) throw error
    throw new Error(typeof error === 'string' ? error : 'O núcleo local recusou a operação.')
  }
}

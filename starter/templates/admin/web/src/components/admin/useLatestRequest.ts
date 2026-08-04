import { useCallback, useEffect, useRef } from 'react'

type RequestLease = Readonly<{
  signal: AbortSignal
  isCurrent: () => boolean
  release: () => void
}>

export default function useLatestRequest() {
  const active = useRef<AbortController | null>(null)

  const cancel = useCallback(() => {
    active.current?.abort()
    active.current = null
  }, [])

  const begin = useCallback((): RequestLease => {
    cancel()
    const controller = new AbortController()
    active.current = controller
    return {
      signal: controller.signal,
      isCurrent: () => active.current === controller && !controller.signal.aborted,
      release: () => {
        if (active.current === controller) active.current = null
      },
    }
  }, [cancel])

  useEffect(() => cancel, [cancel])
  return { begin, cancel }
}

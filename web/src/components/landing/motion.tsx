import { useEffect, useState, type RefObject } from 'react'

/**
 * True when the visitor asked for less motion, or when there is no way to
 * ask — jsdom has no matchMedia, and a test has no use for animation either.
 */
export function prefersStillness(): boolean {
  return (
    typeof window.matchMedia !== 'function' ||
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

/**
 * Reveals every `[data-reveal]` beneath root as it scrolls into view, once.
 *
 * The hidden starting state only applies under `.lp-js`, which this adds. A
 * page that never runs it — or a browser without IntersectionObserver —
 * shows everything where it stands rather than a column of blank sections.
 */
export function useReveal(root: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = root.current
    if (!el || typeof IntersectionObserver === 'undefined') {
      return
    }

    el.classList.add('lp-js')
    const targets = el.querySelectorAll<HTMLElement>('[data-reveal]')

    if (prefersStillness()) {
      targets.forEach((t) => t.classList.add('is-in'))
      return
    }

    // A clip reveal starts fully clipped, and Chrome counts a fully clipped
    // element as never intersecting. Those are watched through their parent
    // instead, which is laid out in the same place and never clipped.
    const watched = new Map<Element, HTMLElement[]>()
    targets.forEach((t) => {
      const key = t.dataset.reveal === 'clip' && t.parentElement ? t.parentElement : t
      watched.set(key, [...(watched.get(key) ?? []), t])
    })

    const io = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            watched.get(entry.target)?.forEach((t) => t.classList.add('is-in'))
            io.unobserve(entry.target)
          }
        }
      },
      { threshold: 0.12, rootMargin: '0px 0px -6% 0px' },
    )

    watched.forEach((_, key) => io.observe(key))
    return () => io.disconnect()
  }, [root])
}

/**
 * Scroll state for the nav: whether the page has left the top, how far down
 * it is, and which of the given sections is in the middle of the screen.
 */
export function useScrollSpy(ids: string[], progress: RefObject<HTMLDivElement | null>) {
  const [scrolled, setScrolled] = useState(false)
  const [current, setCurrent] = useState<string | null>(null)

  useEffect(() => {
    let frame = 0
    const update = () => {
      frame = 0
      const max = document.documentElement.scrollHeight - window.innerHeight
      setScrolled(window.scrollY > 40)
      if (progress.current) {
        progress.current.style.transform = `scaleX(${max > 0 ? window.scrollY / max : 0})`
      }
      const mid = window.innerHeight * 0.45
      let found: string | null = null
      for (const id of ids) {
        const r = document.getElementById(id)?.getBoundingClientRect()
        if (r && r.top <= mid && r.bottom > mid) found = id
      }
      setCurrent(found)
    }
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update)
    }
    update()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      cancelAnimationFrame(frame)
    }
  }, [ids, progress])

  return { scrolled, current }
}

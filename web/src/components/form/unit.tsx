import type { ReactNode } from 'react'

/** Field label plus a screen-reader copy of the unit shown inside the field ("流量 (GB)"). */
export function labelWithUnit(label: ReactNode, unit: ReactNode) {
  return unit ? (
    <>
      {label} <span className="sr-only">({unit})</span>
    </>
  ) : (
    label
  )
}

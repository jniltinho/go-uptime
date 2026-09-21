// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function combineClasses(...inputs) {
  return twMerge(clsx(inputs))
}

import { AlertCircle } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '#/components/ui/alert'

export function PageAlert({
  title,
  message,
}: {
  title: string
  message: string
}) {
  return (
    <Alert className="border-rose-500/30 bg-rose-500/10 text-rose-950 dark:text-rose-100">
      <AlertCircle className="size-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}

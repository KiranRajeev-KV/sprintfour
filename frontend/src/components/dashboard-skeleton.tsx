import { Card, CardContent, CardHeader } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'

export function DashboardSkeleton() {
  return (
    <div className="page-wrap space-y-6 px-4 py-8 lg:px-0 lg:py-10">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-6">
        {Array.from({ length: 6 }).map((_, index) => (
          <Card key={index} className="island-shell border-white/45 py-0">
            <CardHeader className="space-y-3 px-5 py-5">
              <Skeleton className="h-3 w-24 rounded-full" />
              <Skeleton className="h-10 w-20" />
            </CardHeader>
            <CardContent className="px-5 pb-5">
              <Skeleton className="h-4 w-full" />
            </CardContent>
          </Card>
        ))}
      </div>
      <Card className="island-shell border-white/45 py-0">
        <CardHeader className="space-y-4 px-5 py-5">
          <Skeleton className="h-6 w-56" />
          <Skeleton className="h-10 w-full rounded-full" />
        </CardHeader>
        <CardContent className="space-y-4 px-5 pb-5">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-12 w-full" />
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

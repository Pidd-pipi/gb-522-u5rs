export const EVENT_TYPES = ['connector', 'splice', 'bend', 'break', 'end', 'unknown'] as const
export type EventType = (typeof EVENT_TYPES)[number]

export interface EventRevision {
  id: number
  event_id: number
  revision_no: number
  previous_event_type: EventType
  previous_distance_m: number
  previous_review_note: string
  previous_reviewed_by?: number
  previous_reviewed_at?: string
  new_event_type: EventType
  new_distance_m: number
  new_review_note: string
  revised_by: number
  created_at: string
}

export interface EventMarker {
  id: number
  trace_id: number
  distance_m: number
  event_type: EventType
  insertion_loss_db: number
  reflectance_db: number
  confidence: number
  algorithm_event_type: EventType
  algorithm_distance_m: number
  algorithm_insertion_loss_db: number
  reviewed: boolean
  review_note: string
  reviewed_by?: number
  reviewed_at?: string
  revision_count: number
  version: number
  latest_revision?: EventRevision
  created_at: string
}

export const eventLabel: Record<EventType, string> = {
  connector: '连接器', splice: '熔接点', bend: '弯曲', break: '断点', end: '光纤末端', unknown: '未知',
}

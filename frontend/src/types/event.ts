export const EVENT_TYPES = ['connector', 'splice', 'bend', 'break', 'end', 'unknown'] as const
export type EventType = (typeof EVENT_TYPES)[number]

export interface EventRevisionSummary {
  id: number
  revision_no: number
  event_type: EventType
  distance_m: number
  review_note: string
  reviewed_by: number
  reviewer_name: string
  reviewed_at: string
  prev_event_type?: EventType
  prev_distance_m?: number
  prev_review_note?: string
  prev_reviewed_by?: number
  prev_reviewer_name?: string
  prev_reviewed_at?: string
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
  version: number
  revision_count: number
  last_revision_at?: string
  last_revision?: EventRevisionSummary
  created_at: string
}

export interface CaseInvalidation {
  case_id: number
  reason: string
  status: string
  version: number
}

export interface ReviewEventResponse {
  event: EventMarker
  revision: EventRevisionSummary
  invalidated_cases: CaseInvalidation[]
}

export const eventLabel: Record<EventType, string> = {
  connector: '连接器', splice: '熔接点', bend: '弯曲', break: '断点', end: '光纤末端', unknown: '未知',
}

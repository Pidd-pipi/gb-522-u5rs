<script setup lang="ts">
import { ref, watch } from 'vue'
import { eventApi } from '@/api/domain'
import { eventLabel, type EventMarker, type EventRevisionSummary } from '@/types/event'

const props = defineProps<{ modelValue: boolean; event: EventMarker | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const items = ref<EventRevisionSummary[]>([])
const loading = ref(false)
function formatAt(value?: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
watch(() => [props.modelValue, props.event?.id], async ([open]) => {
  if (open && props.event) {
    loading.value = true
    try {
      const { data } = await eventApi.revisions(props.event.id)
      items.value = data.data
    } finally {
      loading.value = false
    }
  } else {
    items.value = []
  }
}, { immediate: true })
</script>

<template>
  <el-drawer :model-value="modelValue" title="事件修订轨迹" size="min(460px, 92vw)" @update:model-value="emit('update:modelValue', $event)">
    <div v-loading="loading" class="revision-trail">
      <p v-if="event" class="trail-head">事件 #{{ event.id }} · 算法原值 {{ eventLabel[event.algorithm_event_type] }} · {{ event.algorithm_distance_m.toFixed(2) }} m · 共 {{ event.revision_count }} 次修订</p>
      <el-timeline>
        <el-timeline-item v-for="rev in items" :key="rev.id" :timestamp="`${rev.reviewer_name} · ${formatAt(rev.reviewed_at)}`" placement="top" type="primary">
          <div class="revision-card">
            <div class="revision-no">第 {{ rev.revision_no }} 次人工判定</div>
            <div class="revision-line"><span class="rev-label">新判定</span><strong>{{ eventLabel[rev.event_type] }}</strong> · {{ rev.distance_m.toFixed(2) }} m</div>
            <div class="revision-line note">{{ rev.review_note }}</div>
            <div v-if="rev.prev_event_type" class="revision-line previous">
              <span class="rev-label">旧判定</span>{{ eventLabel[rev.prev_event_type] }} · {{ rev.prev_distance_m?.toFixed(2) }} m · {{ rev.prev_reviewer_name || `用户#${rev.prev_reviewed_by}` }} · {{ formatAt(rev.prev_reviewed_at) }}
              <span v-if="rev.prev_review_note" class="prev-note">「{{ rev.prev_review_note }}」</span>
            </div>
            <div v-else class="revision-line previous first">首次人工复核，此前仅存在算法原值。</div>
          </div>
        </el-timeline-item>
      </el-timeline>
    </div>
  </el-drawer>
</template>

<style scoped>
.trail-head { margin: 0 0 16px; font-size: 12px; color: var(--text-muted); font-weight: 700; }
.revision-card { padding: 4px 0; }
.revision-no { font-weight: 800; font-size: 13px; margin-bottom: 4px; }
.revision-line { font-size: 12px; color: var(--text-regular); }
.revision-line.note { margin: 3px 0; }
.rev-label { display: inline-block; min-width: 48px; color: var(--text-muted); font-weight: 700; }
.previous { margin-top: 5px; padding: 6px 8px; border-left: 3px solid var(--line-strong); background: var(--surface, #f7f8fa); border-radius: 0 4px 4px 0; }
.previous.first { color: var(--text-muted); }
.prev-note { display: block; margin-top: 2px; color: var(--text-muted); }
</style>

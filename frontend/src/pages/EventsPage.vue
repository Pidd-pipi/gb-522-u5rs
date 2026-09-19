<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Filter, History, Radar } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import EventTypeBadge from '@/components/common/EventTypeBadge.vue'
import ReviewDialog from '@/components/common/ReviewDialog.vue'
import EventRevisionHistory from '@/components/common/EventRevisionHistory.vue'
import { useEventStore } from '@/stores/events'
import { useCaseStore } from '@/stores/cases'
import { useRouteStore } from '@/stores/routes'
import { useAuth } from '@/hooks/useAuth'
import { EVENT_TYPES, eventLabel, type EventMarker, type EventType } from '@/types/event'

const events = useEventStore(); const cases = useCaseStore(); const routes = useRouteStore(); const auth = useAuth(); const dialog = ref(false); const historyOpen = ref(false); const current = ref<EventMarker | null>(null); const busy = ref(false)
const filters = reactive<{ route_id?: number; event_type?: EventType; reviewed?: boolean }>({})
async function search() { await events.fetch({ ...filters, page_size: 100 }) }
function openReview(item: EventMarker) { current.value = item; dialog.value = true }
function openHistory(item: EventMarker) { current.value = item; historyOpen.value = true }
async function review(value: Record<string, unknown>) {
  if (!current.value) return
  busy.value = true
  try {
    const result = await events.review(current.value.id, value as { event_type: EventType; distance_m?: number; review_note: string; version: number })
    dialog.value = false
    if (result.invalidated_cases.length > 0) {
      // Referencing open cases were forced back to draft; refresh their page state.
      await cases.fetch({ page_size: 100 })
      await ElMessageBox.alert(
        `事件修订已保存，引用该事件轨迹的 ${result.invalidated_cases.length} 个未关闭案例已退回草稿并记录失效原因，请重新执行基线分析。`,
        '关联案例需重新分析',
        { type: 'warning', confirmButtonText: '我知道了' },
      )
    } else {
      ElMessage.success('事件复核结果已保存')
    }
  } catch {
    // Version conflict or closed-case rejection: reload so the row and dialog match the server.
    await search()
    const latest = events.items.find((item) => item.id === current.value?.id)
    if (latest) current.value = latest
  } finally {
    busy.value = false
  }
}
function formatAt(value?: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
onMounted(async () => { await Promise.all([routes.fetch({page_size:100}), search()]) })
</script>

<template>
  <PageHeader title="事件复核" eyebrow="EVENT REVIEW" description="对照算法原值修订事件类型、距离和备注；每次修订保留旧判定，形成可追溯闭环。" />
  <section class="content-band">
    <div class="filter-band"><div class="filter-label"><Filter :size="16" /><span>筛选条件</span></div><el-select v-model="filters.route_id" clearable placeholder="全部线路" @change="search"><el-option v-for="route in routes.items" :key="route.id" :label="route.route_code" :value="route.id" /></el-select><el-select v-model="filters.event_type" clearable placeholder="全部类型" @change="search"><el-option v-for="type in EVENT_TYPES" :key="type" :label="eventLabel[type]" :value="type" /></el-select><el-select v-model="filters.reviewed" clearable placeholder="全部状态" @change="search"><el-option label="未复核" :value="false" /><el-option label="已复核" :value="true" /></el-select><span class="toolbar-spacer subtle-count">{{ events.total }} 项事件</span></div>
    <div class="data-surface">
      <el-table v-loading="events.loading" :data="events.items" row-key="id">
        <el-table-column label="类型" width="125"><template #default="scope"><EventTypeBadge :type="scope.row.event_type" :reviewed="scope.row.reviewed" /></template></el-table-column>
        <el-table-column prop="trace_id" label="轨迹" width="85"><template #default="scope">#{{ scope.row.trace_id }}</template></el-table-column>
        <el-table-column prop="distance_m" label="定位距离" width="120"><template #default="scope"><strong>{{ scope.row.distance_m.toFixed(2) }}</strong> m</template></el-table-column>
        <el-table-column label="算法原值" min-width="180"><template #default="scope"><span class="algorithm-value">{{ eventLabel[scope.row.algorithm_event_type as EventType] }} · {{ scope.row.algorithm_distance_m.toFixed(2) }} m</span></template></el-table-column>
        <el-table-column label="插入损耗 dB" width="100"><template #default="scope">{{ scope.row.insertion_loss_db.toFixed(2) }}</template></el-table-column>
        <el-table-column label="置信度" width="100"><template #default="scope">{{ Math.round(scope.row.confidence*100) }}%</template></el-table-column>
        <el-table-column label="修订与最近变化" min-width="240">
          <template #default="scope">
            <div v-if="scope.row.reviewed" class="revision-cell">
              <div class="revision-line"><el-tag size="small" type="warning" effect="plain">修订 {{ scope.row.revision_count }} 次</el-tag><el-button v-if="scope.row.revision_count > 0" link type="primary" size="small" @click="openHistory(scope.row)"><History :size="13" />轨迹</el-button></div>
              <div class="latest-change">{{ eventLabel[scope.row.event_type as EventType] }} · {{ scope.row.distance_m.toFixed(2) }} m</div>
              <div class="latest-meta">{{ formatAt(scope.row.last_revision_at ?? scope.row.reviewed_at) }}</div>
            </div>
            <span v-else class="unreviewed">待人工复核</span>
          </template>
        </el-table-column>
        <el-table-column width="94" fixed="right"><template #default="scope"><el-button v-if="auth.canReview()" link type="primary" @click="openReview(scope.row)">{{ scope.row.reviewed ? '重新复核' : '复核' }}</el-button><span v-else class="muted">只读</span></template></el-table-column>
        <template #empty><div class="empty-state"><div><Radar :size="34" /><strong>没有匹配事件</strong><span>请先在轨迹分析页执行检测。</span></div></div></template>
      </el-table>
    </div>
  </section>
  <ReviewDialog v-model="dialog" mode="event" :event="current" :loading="busy" @submit="review" />
  <EventRevisionHistory v-model="historyOpen" :event="current" />
</template>

<style scoped>
.filter-band{display:flex;align-items:center;flex-wrap:wrap;gap:9px;margin-bottom:14px;padding:11px;border:1px solid var(--line);background:var(--surface)}.filter-band .el-select{width:160px}.filter-label{display:flex;align-items:center;gap:7px;margin-right:5px;color:var(--text-muted);font-size:12px;font-weight:800}.algorithm-value{color:var(--text-muted);font-size:12px}.revision-cell{display:flex;flex-direction:column;gap:2px}.revision-line{display:flex;align-items:center;gap:6px}.latest-change{font-size:12px;font-weight:700}.latest-meta{color:var(--text-muted);font-size:11px}.unreviewed{color:var(--warning);font-size:12px;font-weight:700}.muted{color:var(--text-muted);font-size:12px}@media(max-width:620px){.filter-band .el-select{width:calc(50% - 5px)}.filter-label{width:100%}}
</style>

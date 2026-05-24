<template>
  <TeamManageLayout v-if="teamId" :team-id="teamId" source="team_admin">
    <TeamOverview :team-id="teamId" source="team_admin" />
  </TeamManageLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import TeamManageLayout from '@/components/team-management/TeamManageLayout.vue'
import TeamOverview from '@/components/team-management/TeamOverview.vue'

const route = useRoute()
const teamId = computed(() => {
  const v = Number(route.params.teamId)
  return Number.isFinite(v) && v > 0 ? v : null
})
</script>

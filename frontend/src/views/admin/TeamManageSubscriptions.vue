<template>
  <TeamManageLayout :team-id="teamId" source="admin">
    <TeamSubscriptionsList :team-id="teamId" source="admin" />
  </TeamManageLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import TeamManageLayout from '@/components/team-management/TeamManageLayout.vue'
import TeamSubscriptionsList from '@/components/team-management/TeamSubscriptionsList.vue'

const route = useRoute()
const teamId = computed(() => Number(route.params.id))
</script>

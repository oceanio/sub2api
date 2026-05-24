<template>
  <TeamManageLayout :team-id="teamId" source="admin">
    <TeamTagsManager :team-id="teamId" source="admin" />
  </TeamManageLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import TeamManageLayout from '@/components/team-management/TeamManageLayout.vue'
import TeamTagsManager from '@/components/team-management/TeamTagsManager.vue'

const route = useRoute()
const teamId = computed(() => Number(route.params.id))
</script>

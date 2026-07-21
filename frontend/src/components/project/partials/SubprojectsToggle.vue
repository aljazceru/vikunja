<template>
	<div
		v-if="hasChildProjects"
		class="subprojects-toggle"
	>
		<FancyCheckbox
			v-model="showSubprojectTasks"
			class="subprojects-toggle__checkbox"
		>
			{{ $t('project.showSubprojectTasks.label') }}
		</FancyCheckbox>
	</div>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'

import FancyCheckbox from '@/components/input/FancyCheckbox.vue'

import {useAuthStore} from '@/stores/auth'
import {useProjectStore} from '@/stores/projects'
import {success} from '@/message'
import {i18n} from '@/i18n'

const props = defineProps<{
	projectId: number,
}>()

const authStore = useAuthStore()
const projectStore = useProjectStore()

const hasChildProjects = computed(() => projectStore.getChildProjects(props.projectId).length > 0)

// Local mirror so the checkbox feels instant; persisted to the backend on change.
const showSubprojectTasks = ref(authStore.settings.frontendSettings.showSubprojectTasks)

watch(() => authStore.settings.frontendSettings.showSubprojectTasks, v => {
	showSubprojectTasks.value = v
})

watch(showSubprojectTasks, async value => {
	if (authStore.isLinkShareAuth) return
	if (value === authStore.settings.frontendSettings.showSubprojectTasks) return
	await authStore.saveUserSettings({
		settings: {
			...authStore.settings,
			frontendSettings: {
				...authStore.settings.frontendSettings,
				showSubprojectTasks: value,
				quickAddDefaultReminders: [...(authStore.settings.frontendSettings.quickAddDefaultReminders ?? [])],
			},
		},
		showMessage: false,
	})
	success({message: i18n.global.t('project.showSubprojectTasks.saved')})
})
</script>

<style scoped lang="scss">
.subprojects-toggle {
	display: inline-flex;
	align-items: center;
}
</style>

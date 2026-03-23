<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy } from 'svelte'
  import type { Project, ProjectGoal, ProjectMilestone, TaskItem } from './types'
  import { createProject, deleteProject, getProjectTasks, getProjects, updateProject } from './api'
  const dispatch = createEventDispatcher()
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { dispatch('close') }
  }
  onDestroy(() => window.removeEventListener('keydown', handleKeydown))

  let projects: Project[] = []
  let loading = true
  let phase: 'list' | 'view' | 'edit' = 'list'
  let selectedIdx = -1
  let projectTasks: TaskItem[] = []
  let filterStatus: string = 'all'
  let filterCategory: string = 'all'

  // Edit form
  let form: Project = emptyProject()

  const statuses = ['active', 'paused', 'completed', 'archived']
  const categories = ['development', 'social-media', 'personal', 'business', 'writing', 'research', 'health', 'finance', 'other']
  const colors = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire', 'flamingo']

  function emptyProject(): Project {
    return { name: '', description: '', folder: '', tags: [], status: 'active', color: 'blue', createdAt: new Date().toISOString().split('T')[0], notes: [], taskFilter: '', category: 'other', goals: [], nextAction: '', priority: 0, dueDate: '', timeSpent: 0 }
  }

  function colorVar(c: string): string { return `var(--ctp-${c})` }

  onMount(async () => {
    window.addEventListener('keydown', handleKeydown)
    await loadProjects()
    loading = false
  })

  async function loadProjects() {
    try { projects = (await getProjects()) || [] } catch { projects = [] }
  }

  let formError = ''

  function startCreate() {
    form = emptyProject()
    formError = ''
    selectedIdx = -1
    phase = 'edit'
  }

  function startEdit(idx: number) {
    if (idx < 0 || idx >= projects.length) return
    form = JSON.parse(JSON.stringify(projects[idx]))
    formError = ''
    selectedIdx = idx
    phase = 'edit'
  }

  async function saveForm() {
    formError = ''
    if (!form.name.trim()) { formError = 'Project name is required.'; return }
    try {
      if (selectedIdx >= 0) {
        await updateProject(selectedIdx, JSON.stringify(form))
      } else {
        await createProject(JSON.stringify(form))
      }
      await loadProjects()
      if (selectedIdx >= 0) { await viewProject(selectedIdx) }
      else if (projects.length > 0) { selectedIdx = projects.length - 1; await viewProject(selectedIdx) }
    } catch (e) { console.error('Save failed:', e); formError = 'Failed to save project.' }
  }

  async function doDeleteProject(idx: number) {
    try {
      await deleteProject(idx)
      await loadProjects()
      phase = 'list'
      selectedIdx = -1
    } catch (e) { console.error('Delete failed:', e) }
  }

  async function viewProject(idx: number) {
    selectedIdx = idx
    phase = 'view'
    const p = projects[idx]
    try { projectTasks = (await getProjectTasks(p.taskFilter || '')) || [] } catch { projectTasks = [] }
  }

  async function cycleStatus(idx: number) {
    if (idx < 0 || idx >= projects.length) return
    const i = statuses.indexOf(projects[idx].status)
    projects[idx].status = statuses[(i + 1) % statuses.length] as any
    projects = [...projects]
    try { await updateProject(idx, JSON.stringify(projects[idx])) } catch (e) { console.error(e) }
  }

  function progress(p: Project): number {
    let total = 0, done = 0
    for (const g of p.goals) {
      if (g.milestones.length > 0) {
        for (const m of g.milestones) { total++; if (m.done) done++ }
      } else { total++; if (g.done) done++ }
    }
    return total > 0 ? done / total : 0
  }

  async function toggleMilestone(goalIdx: number, msIdx: number) {
    const p = projects[selectedIdx]
    p.goals[goalIdx].milestones[msIdx].done = !p.goals[goalIdx].milestones[msIdx].done
    // Check if all milestones done -> mark goal done
    const g = p.goals[goalIdx]
    g.done = g.milestones.every(m => m.done)
    projects = [...projects]
    try { await updateProject(selectedIdx, JSON.stringify(p)) } catch (e) { console.error(e) }
  }

  async function toggleGoal(goalIdx: number) {
    const p = projects[selectedIdx]
    p.goals[goalIdx].done = !p.goals[goalIdx].done
    projects = [...projects]
    try { await updateProject(selectedIdx, JSON.stringify(p)) } catch (e) { console.error(e) }
  }

  function addGoal() {
    form.goals.push({ title: '', done: false, milestones: [] })
    form = { ...form }
  }

  function removeGoal(idx: number) {
    form.goals.splice(idx, 1)
    form = { ...form }
  }

  function addMilestone(goalIdx: number) {
    form.goals[goalIdx].milestones.push({ text: '', done: false })
    form = { ...form }
  }

  function removeMilestone(goalIdx: number, msIdx: number) {
    form.goals[goalIdx].milestones.splice(msIdx, 1)
    form = { ...form }
  }

  function addTag() {
    form.tags = [...form.tags, '']
    form = { ...form }
  }

  $: filteredProjects = projects.filter(p => {
    if (filterStatus !== 'all' && p.status !== filterStatus) return false
    if (filterCategory !== 'all' && p.category !== filterCategory) return false
    return true
  })

  $: pendingTasks = projectTasks.filter(t => !t.done)
  $: doneTasks = projectTasks.filter(t => t.done)

  function statusColor(s: string): string {
    switch (s) {
      case 'active': return 'var(--ctp-green)'
      case 'paused': return 'var(--ctp-yellow)'
      case 'completed': return 'var(--ctp-blue)'
      case 'archived': return 'var(--ctp-overlay0)'
      default: return 'var(--ctp-text)'
    }
  }
</script>

<div class="fixed inset-0 z-50 flex justify-center pt-[3%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-[90vw] max-w-5xl h-[88vh] bg-ctp-mantle rounded-xl shadow-overlay flex flex-col overflow-hidden"
    style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent)">

    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-3" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
      <div class="flex items-center gap-3">
        {#if phase !== 'list'}
          <button class="text-[13px] text-ctp-overlay1 hover:text-ctp-text transition-colors" on:click={() => phase = 'list'}>← Back</button>
        {/if}
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M2 4h5l1.5-2H13a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4z" />
        </svg>
        <span class="text-[15px] font-semibold text-ctp-text">
          {phase === 'list' ? 'Projects & Goals' : phase === 'view' && selectedIdx >= 0 ? projects[selectedIdx].name : phase === 'edit' && selectedIdx >= 0 ? 'Edit Project' : 'New Project'}
        </span>
        <span class="text-[12px] text-ctp-overlay1">{projects.length} projects</span>
      </div>
      <div class="flex items-center gap-2">
        {#if phase === 'list'}
          <button class="px-3 py-1 text-[12px] text-ctp-crust bg-ctp-blue rounded-md hover:opacity-90 transition-opacity" on:click={startCreate}>+ New Project</button>
        {/if}
        <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0/50 px-1.5 py-0.5 rounded cursor-pointer" on:click={() => dispatch('close')}>esc</kbd>
      </div>
    </div>

    {#if loading}
      <div class="flex-1 flex items-center justify-center"><span class="text-sm text-ctp-overlay1">Loading...</span></div>

    {:else if phase === 'list'}
      <!-- Filters -->
      <div class="flex items-center gap-3 px-5 py-2" style="border-bottom: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
        <select bind:value={filterStatus} class="bg-ctp-base text-[12px] text-ctp-text rounded px-2 py-1 outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
          <option value="all">All statuses</option>
          {#each statuses as s}<option value={s}>{s}</option>{/each}
        </select>
        <select bind:value={filterCategory} class="bg-ctp-base text-[12px] text-ctp-text rounded px-2 py-1 outline-none" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
          <option value="all">All categories</option>
          {#each categories as c}<option value={c}>{c}</option>{/each}
        </select>
      </div>

      <!-- Project list -->
      <div class="flex-1 overflow-y-auto p-4">
        {#if filteredProjects.length === 0}
          <div class="flex flex-col items-center justify-center h-full gap-2 opacity-60">
            <p class="text-sm text-ctp-overlay1">No projects yet</p>
            <button class="text-[13px] text-ctp-blue hover:underline" on:click={startCreate}>Create your first project</button>
          </div>
        {:else}
          <div class="grid grid-cols-2 gap-3">
            {#each filteredProjects as p, i}
              {@const prog = progress(p)}
              <button class="project-card text-left" on:click={() => viewProject(projects.indexOf(p))}>
                <div class="flex items-center gap-2 mb-2">
                  <div class="w-3 h-3 rounded-full" style="background:{colorVar(p.color)}"></div>
                  <span class="text-[14px] font-semibold text-ctp-text flex-1 truncate">{p.name}</span>
                  <span class="text-[10px] font-medium px-1.5 py-0.5 rounded-full"
                    style="color:{statusColor(p.status)};background:color-mix(in srgb, {statusColor(p.status)} 12%, transparent)">
                    {p.status}
                  </span>
                </div>
                {#if p.description}
                  <p class="text-[12px] text-ctp-overlay1 mb-2 line-clamp-2">{p.description}</p>
                {/if}
                <div class="flex items-center gap-3 text-[11px] text-ctp-overlay0">
                  <span>{p.category}</span>
                  {#if p.goals.length > 0}
                    <span>{p.goals.length} goals</span>
                  {/if}
                  {#if p.dueDate}
                    <span>Due {p.dueDate}</span>
                  {/if}
                </div>
                {#if p.goals.length > 0}
                  <div class="mt-2 h-1.5 bg-ctp-surface0/50 rounded-full overflow-hidden">
                    <div class="h-full rounded-full transition-all" style="width:{Math.round(prog * 100)}%;background:{colorVar(p.color)}"></div>
                  </div>
                  <span class="text-[10px] text-ctp-overlay0 mt-0.5">{Math.round(prog * 100)}%</span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>

    {:else if phase === 'view' && selectedIdx >= 0 && selectedIdx < projects.length}
      {@const p = projects[selectedIdx]}
      <div class="flex-1 overflow-y-auto">
        <div class="max-w-3xl mx-auto px-6 py-6">
          <!-- Project header -->
          <div class="flex items-start gap-3 mb-6">
            <div class="w-4 h-4 rounded-full mt-1.5 flex-shrink-0" style="background:{colorVar(p.color)}"></div>
            <div class="flex-1">
              <div class="flex items-center gap-3">
                <h2 class="text-xl font-bold text-ctp-text">{p.name}</h2>
                <button class="text-[11px] font-medium px-2 py-0.5 rounded-full transition-colors cursor-pointer"
                  style="color:{statusColor(p.status)};background:color-mix(in srgb, {statusColor(p.status)} 12%, transparent)"
                  on:click={() => cycleStatus(selectedIdx)}>{p.status}</button>
              </div>
              {#if p.description}
                <p class="text-[13px] text-ctp-overlay1 mt-1">{p.description}</p>
              {/if}
              <div class="flex items-center gap-4 mt-2 text-[12px] text-ctp-overlay0">
                <span>{p.category}</span>
                {#if p.dueDate}<span>Due {p.dueDate}</span>{/if}
                {#if p.taskFilter}<span>Filter: {p.taskFilter}</span>{/if}
                <span>Created {p.createdAt}</span>
              </div>
            </div>
            <button class="text-[12px] text-ctp-overlay1 hover:text-ctp-blue px-2 py-1" on:click={() => startEdit(selectedIdx)}>Edit</button>
            <button class="text-[12px] text-ctp-overlay1 hover:text-ctp-red px-2 py-1" on:click={() => doDeleteProject(selectedIdx)}>Delete</button>
          </div>

          {#if p.nextAction}
            <div class="mb-6 px-4 py-3 rounded-lg" style="background:color-mix(in srgb, {colorVar(p.color)} 8%, transparent);border:1px solid color-mix(in srgb, {colorVar(p.color)} 20%, transparent)">
              <div class="text-[11px] text-ctp-overlay0 mb-1 font-medium">NEXT ACTION</div>
              <div class="text-[14px] text-ctp-text">{p.nextAction}</div>
            </div>
          {/if}

          <!-- Progress -->
          {#if p.goals.length > 0}
            {@const prog = progress(p)}
            <div class="mb-6">
              <div class="flex items-center justify-between mb-2">
                <span class="text-[13px] font-semibold text-ctp-subtext1">Progress</span>
                <span class="text-[12px] text-ctp-overlay0">{Math.round(prog * 100)}%</span>
              </div>
              <div class="h-2 bg-ctp-surface0/40 rounded-full overflow-hidden">
                <div class="h-full rounded-full transition-all" style="width:{Math.round(prog * 100)}%;background:{colorVar(p.color)}"></div>
              </div>
            </div>
          {/if}

          <!-- Goals -->
          <div class="mb-6">
            <h3 class="text-[13px] font-semibold text-ctp-subtext1 mb-3">Goals ({p.goals.length})</h3>
            {#if p.goals.length === 0}
              <p class="text-[12px] text-ctp-overlay0">No goals defined. Edit to add goals.</p>
            {:else}
              <div class="space-y-3">
                {#each p.goals as goal, gi}
                  <div class="rounded-lg p-3" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
                    <button class="flex items-center gap-2 w-full text-left" on:click={() => toggleGoal(gi)}>
                      <span class="w-4 h-4 rounded flex items-center justify-center flex-shrink-0 text-[10px]"
                        class:bg-ctp-green={goal.done} class:text-ctp-crust={goal.done}
                        style={!goal.done ? `border: 1.5px solid var(--ctp-surface2)` : ''}>
                        {goal.done ? '✓' : ''}
                      </span>
                      <span class="text-[13px] font-medium text-ctp-text" class:line-through={goal.done} class:opacity-50={goal.done}>{goal.title}</span>
                      {#if goal.milestones.length > 0}
                        <span class="text-[11px] text-ctp-overlay0 ml-auto">{goal.milestones.filter(m => m.done).length}/{goal.milestones.length}</span>
                      {/if}
                    </button>
                    {#if goal.milestones.length > 0}
                      <div class="ml-6 mt-2 space-y-1">
                        {#each goal.milestones as ms, mi}
                          <button class="flex items-center gap-2 w-full text-left py-0.5" on:click={() => toggleMilestone(gi, mi)}>
                            <span class="w-3 h-3 rounded-sm flex items-center justify-center flex-shrink-0 text-[8px]"
                              class:bg-ctp-green={ms.done} class:text-ctp-crust={ms.done}
                              style={!ms.done ? `border: 1px solid var(--ctp-surface2)` : ''}>
                              {ms.done ? '✓' : ''}
                            </span>
                            <span class="text-[12px] text-ctp-subtext0" class:line-through={ms.done} class:opacity-40={ms.done}>{ms.text}</span>
                          </button>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>

          <!-- Tasks -->
          {#if p.taskFilter}
            <div class="mb-6">
              <h3 class="text-[13px] font-semibold text-ctp-subtext1 mb-3">Tasks ({pendingTasks.length} open, {doneTasks.length} done)</h3>
              {#each pendingTasks.slice(0, 20) as task}
                <div class="flex items-center gap-2 py-1.5 px-2 rounded hover:bg-ctp-surface0/20 transition-colors">
                  <span class="w-3 h-3 rounded-sm flex-shrink-0" style="border: 1px solid var(--ctp-surface2)"></span>
                  <span class="text-[13px] text-ctp-text flex-1 truncate">{task.text}</span>
                  <button class="text-[11px] text-ctp-overlay0 hover:text-ctp-blue" on:click={() => dispatch('openNote', task.notePath)}>
                    {task.notePath.replace(/\.md$/, '').split('/').pop()}
                  </button>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>

    {:else if phase === 'edit'}
      <div class="flex-1 overflow-y-auto">
        <div class="max-w-2xl mx-auto px-6 py-6 space-y-4">
          <!-- Name -->
          <div>
            <label class="form-label">Project Name</label>
            <input class="form-input" bind:value={form.name} placeholder="My Project" />
          </div>

          <!-- Description -->
          <div>
            <label class="form-label">Description</label>
            <textarea class="form-input h-20 resize-none" bind:value={form.description} placeholder="What is this project about?"></textarea>
          </div>

          <div class="grid grid-cols-3 gap-4">
            <!-- Category -->
            <div>
              <label class="form-label">Category</label>
              <select class="form-input" bind:value={form.category}>
                {#each categories as c}<option value={c}>{c}</option>{/each}
              </select>
            </div>
            <!-- Status -->
            <div>
              <label class="form-label">Status</label>
              <select class="form-input" bind:value={form.status}>
                {#each statuses as s}<option value={s}>{s}</option>{/each}
              </select>
            </div>
            <!-- Color -->
            <div>
              <label class="form-label">Color</label>
              <div class="flex flex-wrap gap-1.5 mt-1">
                {#each colors as c}
                  <button class="w-5 h-5 rounded-full transition-transform" style="background:{colorVar(c)}"
                    class:ring-2={form.color === c} class:ring-ctp-text={form.color === c} class:scale-110={form.color === c}
                    on:click={() => form.color = c}></button>
                {/each}
              </div>
            </div>
          </div>

          <div class="grid grid-cols-3 gap-4">
            <div>
              <label class="form-label">Due Date</label>
              <input class="form-input" type="date" bind:value={form.dueDate} />
            </div>
            <div>
              <label class="form-label">Task Filter Tag</label>
              <input class="form-input" bind:value={form.taskFilter} placeholder="#myproject" />
            </div>
            <div>
              <label class="form-label">Folder</label>
              <input class="form-input" bind:value={form.folder} placeholder="projects/myproj" />
            </div>
          </div>

          <div>
            <label class="form-label">Next Action</label>
            <input class="form-input" bind:value={form.nextAction} placeholder="What's the next thing to do?" />
          </div>

          <!-- Goals -->
          <div>
            <div class="flex items-center justify-between mb-2">
              <label class="form-label mb-0">Goals</label>
              <button class="text-[12px] text-ctp-blue hover:underline" on:click={addGoal}>+ Add Goal</button>
            </div>
            {#each form.goals as goal, gi}
              <div class="mb-3 p-3 rounded-lg" style="border: 1px solid color-mix(in srgb, var(--ctp-surface0) 40%, transparent)">
                <div class="flex items-center gap-2 mb-2">
                  <input class="form-input flex-1" bind:value={goal.title} placeholder="Goal title" />
                  <button class="text-[11px] text-ctp-red hover:underline" on:click={() => removeGoal(gi)}>Remove</button>
                </div>
                <div class="ml-4 space-y-1">
                  {#each goal.milestones as ms, mi}
                    <div class="flex items-center gap-2">
                      <input class="form-input flex-1 text-[12px]" bind:value={ms.text} placeholder="Milestone" />
                      <button class="text-[10px] text-ctp-red" on:click={() => removeMilestone(gi, mi)}>×</button>
                    </div>
                  {/each}
                  <button class="text-[11px] text-ctp-overlay1 hover:text-ctp-blue" on:click={() => addMilestone(gi)}>+ Add Milestone</button>
                </div>
              </div>
            {/each}
          </div>

          <!-- Save -->
          <div class="flex items-center justify-end gap-2 pt-4" style="border-top: 1px solid color-mix(in srgb, var(--ctp-surface0) 30%, transparent)">
            {#if formError}
              <span class="text-[12px] text-ctp-red mr-auto">{formError}</span>
            {/if}
            <button class="px-4 py-1.5 text-[13px] text-ctp-overlay1 hover:text-ctp-text" on:click={() => phase = selectedIdx >= 0 ? 'view' : 'list'}>Cancel</button>
            <button class="px-4 py-1.5 text-[13px] text-ctp-crust bg-ctp-blue rounded-md hover:opacity-90" on:click={saveForm}>
              {selectedIdx >= 0 ? 'Save Changes' : 'Create Project'}
            </button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .project-card {
    padding: 1rem 1.25rem;
    border-radius: 0.5rem;
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 35%, transparent);
    background: color-mix(in srgb, var(--ctp-base) 60%, transparent);
    transition: all 100ms;
  }
  .project-card:hover {
    border-color: color-mix(in srgb, var(--ctp-surface0) 60%, transparent);
    background: color-mix(in srgb, var(--ctp-surface0) 15%, transparent);
  }
  .form-label {
    display: block;
    font-size: 12px;
    font-weight: 500;
    color: var(--ctp-overlay1);
    margin-bottom: 0.375rem;
  }
  .form-input {
    width: 100%;
    background: color-mix(in srgb, var(--ctp-base) 80%, transparent);
    border: 1px solid color-mix(in srgb, var(--ctp-surface0) 50%, transparent);
    border-radius: 0.375rem;
    padding: 0.375rem 0.625rem;
    font-size: 13px;
    color: var(--ctp-text);
    outline: none;
    transition: border-color 100ms;
  }
  .form-input:focus { border-color: var(--ctp-blue); }
  .form-input::placeholder { color: var(--ctp-overlay0); }
  select.form-input { cursor: pointer; }
</style>

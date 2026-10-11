import { createRouter, createWebHistory } from 'vue-router'

export default createRouter({
  history: createWebHistory(),
  scrollBehavior: (_to, _from, saved) => saved || { top: 0 },
  routes: [
    { path: '/', name: 'overview', component: () => import('../pages/OverviewPage.vue') },
    { path: '/stacks', name: 'stacks', component: () => import('../pages/StacksGrid.vue') },
    {
      path: '/stacks/:path+',
      name: 'stack-detail',
      component: () => import('../pages/StackDetail.vue'),
    },
    {
      path: '/stack/:path+',
      redirect: (to) => ({ name: 'stack-detail', params: to.params, query: to.query }),
    },
    {
      path: '/operations',
      name: 'operations',
      component: () => import('../pages/OperationsPage.vue'),
    },
    {
      path: '/operations/:id',
      name: 'operation-detail',
      component: () => import('../pages/OperationPage.vue'),
    },
    {
      path: '/repository',
      name: 'repository',
      component: () => import('../pages/RepositoryPage.vue'),
    },
    { path: '/system', name: 'system', component: () => import('../pages/SystemPage.vue') },
    { path: '/:pathMatch(.*)*', component: () => import('../pages/NotFound.vue') },
  ],
})

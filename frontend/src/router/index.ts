import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'stacks',
      component: () => import('../pages/StacksGrid.vue'),
    },
    {
      path: '/stack/:path+',
      name: 'stack-detail',
      component: () => import('../pages/StackDetail.vue'),
      props: true,
    },
  ],
})

export default router

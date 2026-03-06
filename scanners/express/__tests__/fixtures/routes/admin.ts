import { Router } from 'express';

const router = Router();

// Routes with function-call middleware (e.g., requireRole('admin'))
router.get('/admin/users', requireAuth, requireRole('admin'), listAllUsers);
router.post('/admin/users', requireAuth, requireRole('admin'), rateLimiter, createAdminUser);
router.delete('/admin/users/:id', requireAuth, requireRole('superadmin'), deleteAdminUser);

export default router;

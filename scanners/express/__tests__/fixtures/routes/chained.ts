import { Router } from 'express';

const router = Router();

// Route chaining pattern
router.route('/posts/:id')
  .get(getPost)
  .put(requireAuth, updatePost)
  .delete(requireAuth, requireRole('admin'), deletePost);

router.route('/posts')
  .get(listPosts)
  .post(requireAuth, validateBody, createPost);

export default router;

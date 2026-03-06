import { oc } from '@orpc/contract';
import { z } from 'zod';

const RegisterInputSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
  name: z.string(),
});

const AuthResponseSchema = z.object({
  token: z.string(),
  user: z.object({
    id: z.string(),
    email: z.string(),
    name: z.string(),
  }),
});

export const registerAuth = oc
  .route({
    method: 'POST',
    path: '/auth/register',
    summary: 'Register a new user',
    tags: ['Auth'],
    successStatus: 201,
  })
  .input(RegisterInputSchema)
  .output(AuthResponseSchema);

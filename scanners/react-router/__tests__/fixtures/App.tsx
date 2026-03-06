import React from 'react';
import { Route, Navigate } from 'react-router-dom';

// Components (stubs for parsing)
const Home = () => <div>Home</div>;
const Dashboard = () => <div>Dashboard</div>;
const UserProfile = () => <div>UserProfile</div>;
const SettingsLayout = () => <div>Settings</div>;
const Profile = () => <div>Profile</div>;
const Billing = () => <div>Billing</div>;
const NotFound = () => <div>Not Found</div>;
const Login = () => <div>Login</div>;
const MainApp = () => <div>Main</div>;
const AdminPanel = () => <div>Admin</div>;
const AdminUsers = () => <div>AdminUsers</div>;

export function AppRoutes({ user }: { user: any }) {
  return (
    <>
      {/* Basic route */}
      <Route path="/dashboard" element={<Dashboard />} />

      {/* Dynamic segment */}
      <Route path="/users/:id" element={<UserProfile />} />

      {/* Index route (top-level) */}
      <Route index element={<Home />} />

      {/* Nested routes (layout) */}
      <Route path="/settings" element={<SettingsLayout />}>
        <Route path="profile" element={<Profile />} />
        <Route path="billing" element={<Billing />} />
      </Route>

      {/* Catch-all */}
      <Route path="*" element={<NotFound />} />

      {/* Login */}
      <Route path="/login" element={<Login />} />

      {/* Protected routes via ternary with Navigate */}
      {user ? (
        <Route path="/admin" element={<AdminPanel />}>
          <Route path="users" element={<AdminUsers />} />
        </Route>
      ) : (
        <Navigate to="/login" />
      )}
    </>
  );
}

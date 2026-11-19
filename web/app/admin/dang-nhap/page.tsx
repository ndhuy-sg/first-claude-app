import type { Metadata } from "next";
import { LoginForm } from "@/components/admin/LoginForm";
import { ClientOnly } from "@/components/ui/ClientOnly";

export const metadata: Metadata = { title: "Đăng nhập quản trị" };

export default function LoginPage() {
  return (
    <ClientOnly reason="Login form talks to the API from the browser" fallback={<div className="min-h-screen bg-navy" />}>
      <LoginForm />
    </ClientOnly>
  );
}

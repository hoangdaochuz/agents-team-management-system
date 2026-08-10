import { Link } from "react-router-dom";
import type { Role } from "../../api/types";
import { EmptyState } from "../ui";

/** Shown when a user lacks the role for a route (admin / sysadmin). */
export function NoAccess({ role }: { role?: Role }) {
  return (
    <EmptyState
      icon="lock"
      title="No access"
      hint={
        role
          ? `You need the ${role} role for this workspace to view this page.`
          : "You don't have permission to view this page."
      }
      action={
        <Link to="/dashboard" className="btn btn-ghost btn-sm">
          Back to dashboard
        </Link>
      }
    />
  );
}

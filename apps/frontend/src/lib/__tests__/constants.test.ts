import { describe, it, expect } from "vitest";
import {
  ROLE_PORTALS,
  getRolePortal,
  isRoleAllowedRoute,
  resolvePostAuthRedirect,
} from "../constants";

describe("Portal Role Mapping & Redirection", () => {
  describe("getRolePortal", () => {
    it("returns /exams for TEACHER", () => {
      expect(getRolePortal("TEACHER")).toBe(ROLE_PORTALS.TEACHER);
    });

    it("returns /exams for ADMIN", () => {
      expect(getRolePortal("ADMIN")).toBe(ROLE_PORTALS.TEACHER);
    });

    it("returns /sessions for STUDENT / Candidate", () => {
      expect(getRolePortal("STUDENT")).toBe(ROLE_PORTALS.STUDENT);
    });

    it("returns /sessions for PROCTOR", () => {
      expect(getRolePortal("PROCTOR")).toBe(ROLE_PORTALS.STUDENT);
    });

    it("falls back to candidate portal for unknown/empty role", () => {
      expect(getRolePortal(null)).toBe(ROLE_PORTALS.STUDENT);
      expect(getRolePortal(undefined)).toBe(ROLE_PORTALS.STUDENT);
    });
  });

  describe("isRoleAllowedRoute", () => {
    it("allows teacher to access /exams and child routes", () => {
      expect(isRoleAllowedRoute("TEACHER", "/exams")).toBe(true);
      expect(isRoleAllowedRoute("TEACHER", "/exams/123")).toBe(true);
      expect(isRoleAllowedRoute("TEACHER", "/exams/new")).toBe(true);
    });

    it("blocks teacher from student /sessions", () => {
      expect(isRoleAllowedRoute("TEACHER", "/sessions")).toBe(false);
      expect(isRoleAllowedRoute("TEACHER", "/sessions/abc")).toBe(false);
    });

    it("allows candidate to access /sessions and child routes", () => {
      expect(isRoleAllowedRoute("STUDENT", "/sessions")).toBe(true);
      expect(isRoleAllowedRoute("STUDENT", "/sessions/session-456")).toBe(true);
    });

    it("blocks candidate from /exams", () => {
      expect(isRoleAllowedRoute("STUDENT", "/exams")).toBe(false);
      expect(isRoleAllowedRoute("STUDENT", "/exams/123")).toBe(false);
    });
  });

  describe("resolvePostAuthRedirect", () => {
    it("routes teacher directly to /exams if returnUrl is null", () => {
      expect(resolvePostAuthRedirect("TEACHER", null)).toBe("/exams");
    });

    it("routes candidate directly to /sessions if returnUrl is null", () => {
      expect(resolvePostAuthRedirect("STUDENT", null)).toBe("/sessions");
    });

    it("preserves returnUrl if permissible for teacher", () => {
      expect(resolvePostAuthRedirect("TEACHER", "/exams/exam-99")).toBe("/exams/exam-99");
    });

    it("preserves returnUrl if permissible for candidate", () => {
      expect(resolvePostAuthRedirect("STUDENT", "/sessions/room-77")).toBe("/sessions/room-77");
    });

    it("redirects candidate to /sessions if returnUrl was for /exams", () => {
      expect(resolvePostAuthRedirect("STUDENT", "/exams/123")).toBe("/sessions");
    });

    it("redirects teacher to /exams if returnUrl was for /sessions", () => {
      expect(resolvePostAuthRedirect("TEACHER", "/sessions/456")).toBe("/exams");
    });

    it("redirects to portal if returnUrl is / or /login", () => {
      expect(resolvePostAuthRedirect("TEACHER", "/")).toBe("/exams");
      expect(resolvePostAuthRedirect("TEACHER", "/login")).toBe("/exams");
      expect(resolvePostAuthRedirect("STUDENT", "/")).toBe("/sessions");
      expect(resolvePostAuthRedirect("STUDENT", "/login?mode=register")).toBe("/sessions");
    });

    it("prevents open redirect attempts with external or protocol-relative URLs", () => {
      expect(resolvePostAuthRedirect("TEACHER", "https://malicious.com")).toBe("/exams");
      expect(resolvePostAuthRedirect("STUDENT", "//malicious.com")).toBe("/sessions");
      expect(resolvePostAuthRedirect("TEACHER", "/\\malicious.com")).toBe("/exams");
    });
  });
});

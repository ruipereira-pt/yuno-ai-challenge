import { NextResponse } from "next/server";

const API_BASE_URL = process.env.API_BASE_URL || "http://localhost:8080";

export async function GET() {
  try {
    const res = await fetch(`${API_BASE_URL}/health`, { cache: "no-store" });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch (err) {
    return NextResponse.json(
      {
        error: {
          code: "upstream_unavailable",
          message: "Failed to query backend /health",
          details: { reason: String(err) }
        }
      },
      { status: 502 }
    );
  }
}

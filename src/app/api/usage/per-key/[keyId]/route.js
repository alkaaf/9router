import { NextResponse } from "next/server";
import { getUsageStats, getChartData, getUsageHistory } from "@/lib/usageDb";
import { getApiKeyById } from "@/lib/db/repos/apiKeysRepo";

const VALID_PERIODS = new Set(["today", "24h", "7d", "30d", "60d", "all"]);

export const dynamic = "force-dynamic";

export async function GET(request, { params }) {
  try {
    const { keyId } = await params;
    const { searchParams } = new URL(request.url);
    const period = searchParams.get("period") || "7d";

    if (!VALID_PERIODS.has(period)) {
      return NextResponse.json({ error: "Invalid period" }, { status: 400 });
    }

    const key = await getApiKeyById(keyId);
    if (!key) {
      return NextResponse.json({ error: "API key not found" }, { status: 404 });
    }

    const [stats, chartData, history] = await Promise.all([
      getUsageStats(period, { apiKey: key.key }),
      getChartData(period, { apiKey: key.key }),
      getUsageHistory({ apiKey: key.key }),
    ]);

    const byModel = Object.entries(stats.byModel || {}).map(([name, data]) => ({
      name,
      ...data,
    }));

    return NextResponse.json({
      keyId: key.id,
      keyName: key.name,
      keyMasked: key.key.slice(0, 8) + "..." + key.key.slice(-4),
      period,
      stats: {
        totalRequests: stats.totalRequests,
        totalPromptTokens: stats.totalPromptTokens,
        totalCompletionTokens: stats.totalCompletionTokens,
        totalCost: stats.totalCost,
      },
      byModel,
      chartData,
      history,
    });
  } catch (error) {
    console.error("[API] Failed to get per-key usage:", error);
    return NextResponse.json({ error: "Failed to fetch per-key usage" }, { status: 500 });
  }
}

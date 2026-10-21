<?php

namespace App\Http\Controllers;

use App\Models\CollatechAgente;
use App\Models\CollatechTrabajo;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Carbon;

/**
 * Las rutas a las que llama CollaTech Agent. Tu aplicacion no las usa: para
 * imprimir, usa App\Services\Collatech.
 */
class CollatechAgenteController extends Controller
{
    /** Cuantos trabajos se entregan por vuelta. El agente pregunta cada 3 s. */
    private const POR_VUELTA = 20;

    /** GET /jobs: lo que este local tiene pendiente de imprimir. */
    public function trabajos(Request $request): JsonResponse
    {
        $agente = $this->agente($request);

        // Lo que caduco mientras el agente estaba apagado no se imprime: una
        // comanda que sale tres horas tarde hace mas dano que bien.
        $agente->trabajos()
            ->where('estado', CollatechTrabajo::PENDIENTE)
            ->whereNotNull('caduca_en')
            ->where('caduca_en', '<', now())
            ->update([
                'estado' => CollatechTrabajo::CADUCADO,
                'error' => 'caduco antes de que el local lo recogiera',
                'terminado_en' => now(),
            ]);

        $trabajos = $agente->trabajos()
            ->where('estado', CollatechTrabajo::PENDIENTE)
            ->orderBy('created_at')
            ->limit(self::POR_VUELTA)
            ->get()
            ->map(fn (CollatechTrabajo $t) => [
                'id' => $t->id,
                'type' => $t->tipo,
                'data' => $t->datos,
            ]);

        return response()->json(['jobs' => $trabajos]);
    }

    /** POST /jobs/{id}/status: como fue un trabajo. */
    public function estado(Request $request, string $id): JsonResponse
    {
        $agente = $this->agente($request);
        $datos = $request->validate([
            'status' => 'required|in:received,printed,failed',
            'error' => 'nullable|string',
            'local_jobs' => 'nullable|array',
        ]);

        $trabajo = $agente->trabajos()->whereKey($id)->first();
        if ($trabajo === null) {
            // 404 hace que el agente deje de insistir.
            return response()->json(['ok' => false, 'error' => 'trabajo desconocido'], 404);
        }
        // El agente puede repetir un informe; uno atrasado no deshace uno
        // final.
        if ($trabajo->terminado()) {
            return response()->json(['ok' => true]);
        }

        $trabajo->estado = $datos['status'];
        $trabajo->error = ($datos['error'] ?? null) ?: null;
        $trabajo->detalle = $datos['local_jobs'] ?? null;
        if ($datos['status'] === CollatechTrabajo::RECIBIDO) {
            $trabajo->recibido_en ??= now();
        } else {
            $trabajo->terminado_en = now();
        }
        $trabajo->save();

        return response()->json(['ok' => true]);
    }

    /** POST /heartbeat: el agente sigue encendido, y asi estan sus impresoras. */
    public function latido(Request $request): JsonResponse
    {
        $agente = $this->agente($request);
        $datos = $request->validate([
            'agent' => 'nullable|array',
            'stations' => 'nullable|array',
        ]);

        $info = $datos['agent'] ?? [];
        $agente->ultimo_contacto = now();
        $agente->version = $info['version'] ?? null;
        $agente->equipo = $info['hostname'] ?? null;
        if (! empty($info['started_at'])) {
            $agente->encendido_desde = Carbon::parse($info['started_at']);
        }
        $agente->estaciones = $datos['stations'] ?? [];
        $agente->save();

        return response()->json(['ok' => true]);
    }

    private function agente(Request $request): CollatechAgente
    {
        return $request->attributes->get('collatech_agente');
    }
}

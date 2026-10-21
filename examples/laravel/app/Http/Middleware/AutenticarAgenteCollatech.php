<?php

namespace App\Http\Middleware;

use App\Models\CollatechAgente;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Identifica al agente por su token. Cada local tiene el suyo, asi que un
 * agente solo ve y toca sus propios trabajos.
 */
class AutenticarAgenteCollatech
{
    public function handle(Request $request, Closure $next): Response
    {
        $agente = CollatechAgente::porToken($request->bearerToken());
        if ($agente === null) {
            return response()->json(['ok' => false, 'error' => 'token de agente no valido'], 401);
        }
        $request->attributes->set('collatech_agente', $agente);

        return $next($request);
    }
}

<?php

use App\Http\Controllers\CollatechAgenteController;
use App\Http\Middleware\AutenticarAgenteCollatech;
use Illuminate\Support\Facades\Route;

// Las rutas que llama CollaTech Agent. En el panel del agente, la URL del
// backend es la de tu aplicacion seguida de este prefijo, por ejemplo
// https://mi-app.com/api/collatech
Route::prefix('api/collatech')
    ->middleware(AutenticarAgenteCollatech::class)
    ->group(function () {
        Route::get('jobs', [CollatechAgenteController::class, 'trabajos']);
        Route::post('jobs/{id}/status', [CollatechAgenteController::class, 'estado']);
        Route::post('heartbeat', [CollatechAgenteController::class, 'latido']);
    });

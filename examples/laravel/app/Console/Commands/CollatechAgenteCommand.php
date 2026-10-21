<?php

namespace App\Console\Commands;

use App\Models\CollatechAgente;
use Illuminate\Console\Command;

/**
 * php artisan collatech:agente "Local Centro"   da de alta un local
 * php artisan collatech:agente                  lista los locales y su estado
 */
class CollatechAgenteCommand extends Command
{
    protected $signature = 'collatech:agente {nombre? : nombre del local que se da de alta}';

    protected $description = 'Da de alta un local con CollaTech Agent, o lista los que hay y si estan encendidos';

    public function handle(): int
    {
        $nombre = $this->argument('nombre');
        if ($nombre === null) {
            return $this->listar();
        }

        [$agente, $token] = CollatechAgente::crear($nombre);
        $this->info("Local '{$agente->nombre}' dado de alta.");
        $this->newLine();
        $this->line('En el panel del agente de ese local (http://localhost:18743/panel, pestana Estado):');
        $this->line('  URL del backend:    '.url('api/collatech'));
        $this->line("  Token de este local: {$token}");
        $this->newLine();
        $this->warn('Copia el token ahora: no se guarda y no se puede volver a ver.');

        return self::SUCCESS;
    }

    private function listar(): int
    {
        $filas = CollatechAgente::orderBy('nombre')->get()->map(fn (CollatechAgente $a) => [
            $a->id,
            $a->nombre,
            $a->enLinea() ? 'ENCENDIDO' : 'APAGADO',
            $a->ultimo_contacto?->diffForHumans() ?? 'nunca',
            collect($a->estaciones ?? [])
                ->map(fn ($e) => ($e['name'] ?? '?').': '.(($e['ready'] ?? false) ? 'lista' : 'NO'))
                ->implode(', '),
        ]);
        $this->table(['id', 'local', 'agente', 'ultimo contacto', 'estaciones'], $filas);

        return self::SUCCESS;
    }
}

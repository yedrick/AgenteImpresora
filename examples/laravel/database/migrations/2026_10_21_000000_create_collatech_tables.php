<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        // Un agente es la PC de un local con CollaTech Agent instalado.
        Schema::create('collatech_agentes', function (Blueprint $table) {
            $table->id();
            $table->string('nombre');
            // Solo el hash: si se filtra la base de datos, los tokens no
            // sirven para hacerse pasar por un local.
            $table->string('token_hash', 64)->unique();
            // Lo que cuenta el agente en cada latido.
            $table->timestamp('ultimo_contacto')->nullable();
            $table->string('version')->nullable();
            $table->string('equipo')->nullable();
            $table->timestamp('encendido_desde')->nullable();
            $table->json('estaciones')->nullable();
            $table->timestamps();
        });

        Schema::create('collatech_trabajos', function (Blueprint $table) {
            $table->uuid('id')->primary();
            $table->foreignId('agente_id')->constrained('collatech_agentes')->cascadeOnDelete();
            $table->string('estacion');
            $table->string('tipo')->default('ticket');
            $table->json('datos');
            // pending -> received -> printed | failed. expired si nadie lo
            // recogio a tiempo (el agente estuvo apagado).
            $table->string('estado')->default('pending')->index();
            $table->text('error')->nullable();
            $table->json('detalle')->nullable();
            $table->timestamp('caduca_en')->nullable();
            $table->timestamp('recibido_en')->nullable();
            $table->timestamp('terminado_en')->nullable();
            $table->timestamps();

            $table->index(['agente_id', 'estado']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('collatech_trabajos');
        Schema::dropIfExists('collatech_agentes');
    }
};
